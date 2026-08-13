package awsinventory

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/kernul-io/cloudopt/internal/application/ports"
)

// LiveClients bundles SDK clients for live AWS collection.
type LiveClients struct {
	STS       STSAPI
	EC2Global EC2API
	EC2       EC2ClientFactory
	RDS       RDSClientFactory
}

// LoadLiveClients resolves credentials (optional role assumption) and builds regional factories.
func LoadLiveClients(ctx context.Context, roleARN, externalID string) (*LiveClients, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	if roleARN != "" {
		stsClient := sts.NewFromConfig(cfg)
		provider := stscreds.NewAssumeRoleProvider(stsClient, roleARN, func(o *stscreds.AssumeRoleOptions) {
			if externalID != "" {
				o.ExternalID = aws.String(externalID)
			}
		})
		cfg.Credentials = aws.NewCredentialsCache(provider)
	}
	stsAPI := sts.NewFromConfig(cfg)
	ec2Global := ec2.NewFromConfig(cfg)
	ec2Factory := func(region string) EC2API {
		regCfg := cfg.Copy()
		regCfg.Region = region
		return ec2.NewFromConfig(regCfg)
	}
	rdsFactory := func(region string) RDSAPI {
		regCfg := cfg.Copy()
		regCfg.Region = region
		return rds.NewFromConfig(regCfg)
	}
	return &LiveClients{
		STS:       stsAPI,
		EC2Global: ec2Global,
		EC2:       ec2Factory,
		RDS:       rdsFactory,
	}, nil
}

// NewLiveCollector builds a collector backed by the AWS SDK (read-only APIs only).
func NewLiveCollector(ctx context.Context, roleARN, externalID string) (ports.InventoryCollector, error) {
	clients, err := LoadLiveClients(ctx, roleARN, externalID)
	if err != nil {
		return nil, err
	}
	return NewCollector(clients.STS, clients.EC2Global, clients.EC2, clients.RDS), nil
}
