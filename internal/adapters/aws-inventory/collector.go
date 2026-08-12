package awsinventory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/kernul-io/cloudopt/internal/application/domain"
	"github.com/kernul-io/cloudopt/internal/application/domain/types"
	"github.com/kernul-io/cloudopt/internal/application/ports"
)

const collectorSource = "aws-inventory/collector"

// Collector implements read-only AWS inventory collection.
type Collector struct {
	STS            STSAPI
	EC2Global      EC2API
	EC2ForRegion   EC2ClientFactory
	RDSForRegion   RDSClientFactory
	CapabilitiesFn func() (ports.CapabilityManifest, error)
	Retry          RetryConfig
}

func NewCollector(sts STSAPI, ec2Global EC2API, ec2 EC2ClientFactory, rds RDSClientFactory) *Collector {
	return &Collector{
		STS:          sts,
		EC2Global:    ec2Global,
		EC2ForRegion: ec2,
		RDSForRegion: rds,
		CapabilitiesFn: func() (ports.CapabilityManifest, error) {
			return LoadCapabilities()
		},
		Retry: defaultRetryConfig(),
	}
}

func (c *Collector) Capabilities() ports.CapabilityManifest {
	m, err := c.CapabilitiesFn()
	if err != nil {
		return ports.CapabilityManifest{Provider: types.ProviderAWS}
	}
	return m
}

func (c *Collector) Preflight(ctx context.Context, opts ports.CollectOptions) (*ports.InventoryPreflight, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	caps, err := c.CapabilitiesFn()
	if err != nil {
		return nil, err
	}
	regions, err := c.resolveRegions(ctx, opts)
	if err != nil {
		return nil, err
	}
	identity, err := c.callerIdentity(ctx)
	if err != nil {
		return nil, err
	}
	pf := &ports.InventoryPreflight{
		ProviderAccountID: awsString(identity.Account),
		CallerARN:         awsString(identity.Arn),
		SelectedRegions:   regions,
		ReachableServices: []string{"ec2", "rds", "sts"},
		Capabilities:      caps,
	}
	pf.MissingActions = c.probePermissions(ctx, regions)
	return pf, nil
}

func (c *Collector) Collect(ctx context.Context, opts ports.CollectOptions, progress ports.ProgressReporter) (*domain.CollectionSnapshot, error) {
	if progress == nil {
		progress = ports.NopProgress{}
	}
	pf, err := c.Preflight(ctx, opts)
	if err != nil {
		return nil, err
	}
	if opts.DryRun {
		return nil, nil
	}

	started := types.NowUTC()
	accountID := opts.AccountID
	if accountID == "" {
		accountID = types.AccountID("acct-aws-" + pf.ProviderAccountID)
	}

	regions := pf.SelectedRegions
	progress.Step(fmt.Sprintf("collecting inventory in %d region(s)", len(regions)))

	var (
		mu         sync.Mutex
		regionsOut []domain.Region
		resources  []domain.Resource
		rels       []domain.Relationship
		coverage   []domain.ServiceCollectionStatus
		partial    bool
		idIndex    = map[string]types.ResourceID{}
	)

	maxWorkers := opts.MaxConcurrent
	if maxWorkers <= 0 {
		maxWorkers = 3
	}
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	for _, region := range regions {
		wg.Add(1)
		sem <- struct{}{}
		go func(region string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := ctx.Err(); err != nil {
				return
			}
			progress.Step("region " + region)
			regID := types.RegionID("reg-" + region)
			obs := started
			reg := domain.Region{
				ID:               regID,
				ProviderRegionID: region,
				DisplayName:      region,
				Provenance:       domain.CollectProvenance(collectorSource, obs),
			}

			ec2Client := c.EC2ForRegion(region)
			rdsClient := c.RDSForRegion(region)

			regRes, regRels, regCov, regPartial, err := c.collectRegion(ctx, ec2Client, rdsClient, region, accountID, regID, obs, idIndex)
			mu.Lock()
			defer mu.Unlock()
			regionsOut = append(regionsOut, reg)
			resources = append(resources, regRes...)
			rels = append(rels, regRels...)
			coverage = append(coverage, regCov...)
			if regPartial {
				partial = true
			}
			if err != nil {
				coverage = append(coverage, domain.ServiceCollectionStatus{
					Service: "region",
					Region:  region,
					Status:  domain.ServiceCollectionFailed,
					Message: redactMessage(err.Error()),
				})
				partial = true
			}
		}(region)
	}
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return nil, ErrCancelled
	}

	sort.Slice(regionsOut, func(i, j int) bool {
		return regionsOut[i].ProviderRegionID < regionsOut[j].ProviderRegionID
	})

	status := domain.SnapshotComplete
	if partial {
		status = domain.SnapshotPartial
	}
	completed := types.NowUTC()
	snapID, err := newSnapshotID()
	if err != nil {
		return nil, err
	}

	snap := &domain.CollectionSnapshot{
		ID:            snapID,
		AccountID:     accountID,
		Provider:      types.ProviderAWS,
		Status:        status,
		SchemaVersion: 1,
		StartedAt:     started,
		CompletedAt:   &completed,
		Account: domain.Account{
			ID:                accountID,
			Provider:          types.ProviderAWS,
			ProviderAccountID: pf.ProviderAccountID,
			DisplayName:       "AWS " + pf.ProviderAccountID,
			DefaultCurrency:   "USD",
			Provenance:        domain.CollectProvenance(collectorSource, completed),
		},
		Regions:       regionsOut,
		Resources:     resources,
		Relationships: rels,
		Coverage:      domain.CollectionCoverage{Services: coverage},
	}
	return snap, nil
}

func (c *Collector) collectRegion(
	ctx context.Context,
	ec2 EC2API,
	rds RDSAPI,
	region string,
	accountID types.AccountID,
	regID types.RegionID,
	obs types.Timestamp,
	idIndex map[string]types.ResourceID,
) ([]domain.Resource, []domain.Relationship, []domain.ServiceCollectionStatus, bool, error) {
	var resources []domain.Resource
	var rels []domain.Relationship
	var coverage []domain.ServiceCollectionStatus
	partial := false

	type step struct {
		name string
		fn   func() error
	}
	steps := []step{
		{"ec2_vpc", func() error {
			res, err := c.collectVPCs(ctx, ec2, region, accountID, regID, obs, idIndex)
			if err != nil {
				return err
			}
			resources = append(resources, res...)
			return nil
		}},
		{"ec2_subnet", func() error {
			res, r, err := c.collectSubnets(ctx, ec2, region, accountID, regID, obs, idIndex)
			if err != nil {
				return err
			}
			resources = append(resources, res...)
			rels = append(rels, r...)
			return nil
		}},
		{"ec2_nat", func() error {
			res, r, err := c.collectNAT(ctx, ec2, region, accountID, regID, obs, idIndex)
			if err != nil {
				return err
			}
			resources = append(resources, res...)
			rels = append(rels, r...)
			return nil
		}},
		{"ec2_route_table", func() error {
			res, err := c.collectRouteTables(ctx, ec2, region, accountID, regID, obs, idIndex)
			if err != nil {
				return err
			}
			resources = append(resources, res...)
			return nil
		}},
		{"ec2_eip", func() error {
			res, err := c.collectEIPs(ctx, ec2, region, accountID, regID, obs, idIndex)
			if err != nil {
				return err
			}
			resources = append(resources, res...)
			return nil
		}},
		{"ec2_instance", func() error {
			res, r, typesUsed, err := c.collectInstances(ctx, ec2, region, accountID, regID, obs, idIndex)
			if err != nil {
				return err
			}
			resources = append(resources, res...)
			rels = append(rels, r...)
			if len(typesUsed) > 0 {
				typeRes, err := c.collectInstanceTypes(ctx, ec2, region, accountID, regID, obs, typesUsed, idIndex)
				if err != nil {
					return err
				}
				resources = append(resources, typeRes...)
			}
			return nil
		}},
		{"ec2_volume", func() error {
			res, r, err := c.collectVolumes(ctx, ec2, region, accountID, regID, obs, idIndex)
			if err != nil {
				return err
			}
			resources = append(resources, res...)
			rels = append(rels, r...)
			return nil
		}},
		{"ec2_snapshot", func() error {
			res, err := c.collectSnapshots(ctx, ec2, region, accountID, regID, obs, idIndex)
			if err != nil {
				return err
			}
			resources = append(resources, res...)
			return nil
		}},
		{"rds_instance", func() error {
			res, err := c.collectRDS(ctx, rds, region, accountID, regID, obs, idIndex)
			if err != nil {
				return err
			}
			resources = append(resources, res...)
			return nil
		}},
	}

	for _, st := range steps {
		if err := ctx.Err(); err != nil {
			return resources, rels, coverage, true, ErrCancelled
		}
		if err := st.fn(); err != nil {
			stStatus := domain.ServiceCollectionFailed
			if errorsIsAccessDenied(err) {
				stStatus = domain.ServiceCollectionPartial
			}
			coverage = append(coverage, domain.ServiceCollectionStatus{
				Service: st.name,
				Region:  region,
				Status:  stStatus,
				Message: redactMessage(err.Error()),
			})
			partial = true
			continue
		}
		coverage = append(coverage, domain.ServiceCollectionStatus{
			Service: st.name,
			Region:  region,
			Status:  domain.ServiceCollectionOK,
		})
	}
	return resources, rels, coverage, partial, nil
}

func errorsIsAccessDenied(err error) bool {
	return errors.Is(err, ErrAccessDenied)
}

func (c *Collector) callerIdentity(ctx context.Context) (*sts.GetCallerIdentityOutput, error) {
	var out *sts.GetCallerIdentityOutput
	err := withRetry(ctx, c.Retry, func() error {
		resp, err := c.STS.GetCallerIdentity(ctx, nil)
		if err != nil {
			return mapAWSError("sts", "GetCallerIdentity", "global", err)
		}
		out = resp
		return nil
	}, retryableAPIErr)
	return out, err
}

func (c *Collector) resolveRegions(ctx context.Context, opts ports.CollectOptions) ([]string, error) {
	if len(opts.Regions) > 0 {
		return filterRegions(opts.Regions, opts.RegionsAllow, opts.RegionsDeny), nil
	}
	var names []string
	err := withRetry(ctx, c.Retry, func() error {
		resp, err := c.EC2Global.DescribeRegions(ctx, &ec2.DescribeRegionsInput{AllRegions: boolPtr(false)})
		if err != nil {
			return mapAWSError("ec2", "DescribeRegions", "global", err)
		}
		for _, r := range resp.Regions {
			if r.RegionName != nil {
				names = append(names, *r.RegionName)
			}
		}
		return nil
	}, retryableAPIErr)
	if err != nil {
		return nil, err
	}
	return filterRegions(names, opts.RegionsAllow, opts.RegionsDeny), nil
}

func (c *Collector) probePermissions(ctx context.Context, regions []string) []string {
	if len(regions) == 0 {
		return nil
	}
	region := regions[0]
	client := c.EC2ForRegion(region)
	var missing []string
	_, err := client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{MaxResults: int32Ptr(5)})
	if errorsIsAccessDenied(err) {
		missing = append(missing, "ec2:DescribeInstances")
	}
	return missing
}

func newSnapshotID() (types.SnapshotID, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("snapshot id: %w", err)
	}
	return types.SnapshotID("snap-" + hex.EncodeToString(b[:])), nil
}

func canonicalID(providerID string) types.ResourceID {
	safe := strings.NewReplacer("/", "-", ":", "-").Replace(providerID)
	return types.ResourceID("res-" + safe)
}

func registerID(index map[string]types.ResourceID, providerID string) types.ResourceID {
	id := canonicalID(providerID)
	index[providerID] = id
	return id
}

func lookupID(index map[string]types.ResourceID, providerID string) (types.ResourceID, bool) {
	id, ok := index[providerID]
	return id, ok
}

func awsString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func boolPtr(v bool) *bool { return &v }

func int32Ptr(v int32) *int32 { return &v }
