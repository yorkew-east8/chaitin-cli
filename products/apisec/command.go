package apisec

import (
	"fmt"
	"os"

	"github.com/chaitin/chaitin-cli/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	runtimeCfg       Config
	verbose          bool
	verboseSensitive bool
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apisec",
		Short: "APISec management tool",
		Long: `APISec CLI

Authentication uses the APISec API-TOKEN HTTP header.

Config example:
  apisec:
    url: https://your-apisec.example
    api_token: your-api-token

Command model:
  chaitin-cli apisec raw --help
    Exposes generated raw operations for all available APISec APIs.

  chaitin-cli apisec asset site list --query count=100 --query offset=0 --output json
    Lists APISec site assets without requiring internal FilterAPI scope names.

  chaitin-cli apisec asset api list --query count=100 --query offset=0 --output json
    Lists APISec API assets without requiring internal FilterAPI scope names.

  chaitin-cli apisec asset app --help
    Exposes mapped semantic commands for priority workflows.

For complex JSON input, use --body for inline JSON or --body-file for a JSON file.
Operation help includes endpoint and operation ID so AI agents can map commands back to the API contract.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			applyRuntimeConfig(cmd)
		},
	}

	cmd.PersistentFlags().String("url", "", "APISec API URL")
	cmd.PersistentFlags().String("api-token", "", "APISec API token sent as API-TOKEN header")
	cmd.PersistentFlags().StringP("output", "o", "json", "Output format (table|json)")
	cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Print request URL, headers, and body")
	cmd.PersistentFlags().BoolVar(&verboseSensitive, "verbose-sensitive", false, "Print sensitive values such as API tokens in verbose output")

	if err := loadDynamicCommands(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}
	cmd.AddCommand(newScopesCommand())

	return cmd
}

func newScopesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "scopes",
		Short: "List common APISec FilterAPI scopes",
		RunE: func(cmd *cobra.Command, args []string) error {
			scopes := []map[string]string{
				{"scope": "inventory:app", "description": "application assets"},
				{"scope": "inventory:site", "description": "site assets"},
				{"scope": "inventory:api", "description": "API assets"},
				{"scope": "inventory:detect:split", "description": "predicate split rules"},
				{"scope": "risk:detect:strategy", "description": "risk discovery strategies"},
				{"scope": "risk:risk_event", "description": "risk event groups"},
			}
			return getRenderer(cmd).Render(scopes)
		},
	}
}

func ApplyRuntimeConfig(cmd *cobra.Command, cfg config.Raw, isDryRun bool) {
	productCfg, err := config.DecodeProduct[Config](cfg, "apisec")
	if err != nil {
		return
	}
	runtimeCfg = productCfg
	dryRun = isDryRun
}

func applyRuntimeConfig(cmd *cobra.Command) {
	if flag := lookupFlag(cmd, "url"); flag != nil && !flag.Changed && runtimeCfg.URL != "" {
		_ = setFlag(cmd, "url", runtimeCfg.URL)
	}
	if flag := lookupFlag(cmd, "api-token"); flag != nil && !flag.Changed && runtimeCfg.APIToken != "" {
		_ = setFlag(cmd, "api-token", runtimeCfg.APIToken)
	}
}

func lookupFlag(cmd *cobra.Command, name string) *pflag.Flag {
	if flag := cmd.Flags().Lookup(name); flag != nil {
		return flag
	}
	return cmd.PersistentFlags().Lookup(name)
}

func setFlag(cmd *cobra.Command, name, value string) error {
	if cmd.Flags().Lookup(name) != nil {
		return cmd.Flags().Set(name, value)
	}
	return cmd.PersistentFlags().Set(name, value)
}

func loadDynamicCommands(cmd *cobra.Command) error {
	api, mapping, err := loadEmbeddedSchema()
	if err != nil {
		return err
	}
	parser := NewParser(api, mapping)

	rawCmd := &cobra.Command{
		Use:   "raw",
		Short: "Raw APISec API operations",
		Long:  "Raw APISec API operations generated from the embedded OpenAPI schema. Operation help includes endpoint, operation ID, parameters, and body fallback guidance.",
	}
	rawCommands, err := parser.GenerateRawCommands()
	if err != nil {
		return err
	}
	for _, raw := range rawCommands {
		rawCmd.AddCommand(raw)
	}
	cmd.AddCommand(rawCmd)

	semanticCommands, err := parser.GenerateSemanticCommands()
	if err != nil {
		return err
	}
	for _, semantic := range semanticCommands {
		cmd.AddCommand(semantic)
	}

	return nil
}
