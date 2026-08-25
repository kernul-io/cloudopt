package terraforminput

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kernul-io/cloudopt/internal/application/terraform"
)

const (
	maxStateBytes = 32 << 20 // 32 MiB untrusted input cap
	maxPlanBytes  = 64 << 20
)

// Reader loads Terraform state/plan JSON from disk without executing the Terraform CLI.
type Reader struct{}

func NewReader() *Reader {
	return &Reader{}
}

// LoadStateFile parses terraform show -json state output from path.
func (r *Reader) LoadStateFile(ctx context.Context, path string) ([]terraform.ManagedResource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := readLimited(path, maxStateBytes)
	if err != nil {
		return nil, err
	}
	return parseStateJSON(data)
}

// LoadPlanFile parses terraform show -json plan output from path.
func (r *Reader) LoadPlanFile(ctx context.Context, path string) ([]terraform.PlanResourceChange, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := readLimited(path, maxPlanBytes)
	if err != nil {
		return nil, err
	}
	return parsePlanJSON(data)
}

func readLimited(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open terraform input %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	lim := io.LimitReader(f, max+1)
	data, err := io.ReadAll(lim)
	if err != nil {
		return nil, fmt.Errorf("read terraform input: %w", err)
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("terraform input exceeds %d byte limit", max)
	}
	return data, nil
}

type stateDocument struct {
	Values *stateValues `json:"values"`
}

type stateValues struct {
	RootModule stateModule `json:"root_module"`
}

type stateModule struct {
	Resources    []stateResource `json:"resources"`
	ChildModules []stateModule   `json:"child_modules"`
}

type stateResource struct {
	Address      string                 `json:"address"`
	Mode         string                 `json:"mode"`
	Type         string                 `json:"type"`
	Name         string                 `json:"name"`
	ProviderName string                 `json:"provider_name"`
	Index        json.RawMessage        `json:"index"`
	Values       map[string]interface{} `json:"values"`
	SourceRange  *sourceRange           `json:"source_range"`
}

type sourceRange struct {
	Filename string `json:"filename"`
}

func parseStateJSON(data []byte) ([]terraform.ManagedResource, error) {
	var doc stateDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse state json: %w", err)
	}
	if doc.Values == nil {
		return nil, fmt.Errorf("state json missing values.root_module")
	}
	var out []terraform.ManagedResource
	walkModules(doc.Values.RootModule, &out)
	return out, nil
}

func walkModules(mod stateModule, out *[]terraform.ManagedResource) {
	for _, res := range mod.Resources {
		mr := mapStateResource(res)
		*out = append(*out, mr)
	}
	for _, child := range mod.ChildModules {
		walkModules(child, out)
	}
}

func mapStateResource(res stateResource) terraform.ManagedResource {
	modulePath, _ := terraform.ParseModulePath(res.Address)
	values := flattenValues(res.Values)
	mr := terraform.ManagedResource{
		Address:       res.Address,
		ModulePath:    modulePath,
		Type:          res.Type,
		Name:          res.Name,
		ProviderType:  res.ProviderName,
		ProviderAlias: aliasFromProvider(res.ProviderName),
		Mode:          res.Mode,
		IndexKey:      terraform.IndexKeyFromAddress(res.Address),
		Values:        values,
	}
	if res.SourceRange != nil {
		mr.SourceFile = res.SourceRange.Filename
	}
	return mr
}

func aliasFromProvider(name string) string {
	// registry.terraform.io/hashicorp/aws:alias.prod -> aws.prod
	if idx := strings.LastIndex(name, ":"); idx >= 0 && idx+1 < len(name) {
		return name[idx+1:]
	}
	return ""
}

func flattenValues(in map[string]interface{}) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = stringifyValue(v)
	}
	return out
}

func stringifyValue(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return fmt.Sprintf("%v", t)
	case bool:
		return fmt.Sprintf("%t", t)
	case map[string]interface{}:
		if len(t) == 0 {
			return ""
		}
		// Tags / labels map
		parts := make([]string, 0, len(t))
		for k, val := range t {
			parts = append(parts, fmt.Sprintf("%s=%s", k, stringifyValue(val)))
		}
		return strings.Join(parts, ",")
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return string(b)
	}
}

type planDocument struct {
	ResourceChanges []planResourceChange `json:"resource_changes"`
}

type planResourceChange struct {
	Address string          `json:"address"`
	Type    string          `json:"type"`
	Change  planChangeBlock `json:"change"`
}

type planChangeBlock struct {
	Actions []string               `json:"actions"`
	Before  map[string]interface{} `json:"before"`
	After   map[string]interface{} `json:"after"`
}

func parsePlanJSON(data []byte) ([]terraform.PlanResourceChange, error) {
	var doc planDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse plan json: %w", err)
	}
	out := make([]terraform.PlanResourceChange, 0, len(doc.ResourceChanges))
	for _, rc := range doc.ResourceChanges {
		out = append(out, terraform.PlanResourceChange{
			Address: rc.Address,
			Type:    rc.Type,
			Actions: rc.Change.Actions,
			Before:  flattenValues(rc.Change.Before),
			After:   flattenValues(rc.Change.After),
		})
	}
	return out, nil
}
