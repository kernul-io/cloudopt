package cli

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"

	"github.com/kernul-io/cloudopt/internal/application/capabilities"
	"github.com/kernul-io/cloudopt/internal/domain/types"
)

func newCapabilitiesCommand(_ *Config) *cobra.Command {
	var providersCSV string
	cmd := &cobra.Command{
		Use:   "capabilities",
		Short: "Inspect provider capability manifests and cross-provider matrix",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "matrix",
		Short: "Print the cross-provider capability matrix (JSON on stdout)",
		RunE: func(cmd *cobra.Command, args []string) error {
			all, err := capabilities.AllProviderManifests()
			if err != nil {
				return err
			}
			var filter []types.Provider
			for _, p := range splitCSV(providersCSV) {
				filter = append(filter, types.Provider(p))
			}
			matrix := capabilities.MatrixForScope(all, filter)
			payload := struct {
				Matrix     *capabilities.Matrix          `json:"matrix"`
				Contract   []capabilities.ContractResult `json:"contract_results"`
				Advertised []string                      `json:"advertised_providers"`
			}{
				Matrix:     matrix,
				Contract:   capabilities.RunContractSuite(all),
				Advertised: providerStrings(capabilities.AdvertisedProviders(all)),
			}
			data, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				return err
			}
			data = append(data, '\n')
			_, err = os.Stdout.Write(data)
			return err
		},
	})
	cmd.PersistentFlags().StringVar(&providersCSV, "providers", "", "Comma-separated providers to include (default: all registered)")
	return cmd
}

func providerStrings(in []types.Provider) []string {
	out := make([]string, 0, len(in))
	for _, p := range in {
		out = append(out, string(p))
	}
	return out
}
