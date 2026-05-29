package site

import (
	"bytes"
	"encoding/json"
	"fmt"

	safelinecmd "github.com/chaitin/chaitin-cli/products/safeline/cmd"
	safelineruntime "github.com/chaitin/chaitin-cli/products/safeline/runtime"
	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	opts := createOptions{URLPath: "/", URLPathOp: "pre", BackendType: "proxy", LoadBalance: "Round Robin", XFFAction: "append", RedirectCode: 302}
	var request string
	c := &cobra.Command{
		Use:   "create",
		Short: "Create a SafeLine site",
		RunE: func(c *cobra.Command, args []string) error {
			ctx, err := safelineruntime.ResolveContext(safelinecmd.NewClient(), safelineruntime.Options{
				VersionOverride:       safelinecmd.VersionOverride,
				OperationModeOverride: safelinecmd.OperationModeOverride,
				ConfigVersion:         safelinecmd.ConfigVersion,
				ConfigOperationMode:   safelinecmd.ConfigOperationMode,
			})
			if err != nil {
				return err
			}
			if !ctx.SiteCreateSupported {
				return fmt.Errorf("site create is unsupported for operation mode %q", ctx.OperationMode)
			}
			if request != "" {
				opts.Request = json.RawMessage(request)
			}
			payload, err := buildCreatePayload(ctx, opts)
			if err != nil {
				return err
			}
			warnings, errors := localCreateChecks(payload)
			warnings = append(warnings, ctx.Warnings...)
			check := newCheckResult(ctx.Endpoint, payload, warnings, errors)
			check.Data.RemoteChecks = remoteCreateChecks(safelinecmd.NewClient(), ctx.Endpoint, payload)
			if opts.Check || opts.Explain || safelinecmd.IsDryRun() {
				return safelinecmd.PrintResult(c, check)
			}
			if len(errors) > 0 {
				return fmt.Errorf("site create preflight failed: %v", errors)
			}
			if !opts.Yes {
				return fmt.Errorf("site create is a write operation; re-run with --yes or use --check to preview")
			}
			body, err := json.Marshal(payload)
			if err != nil {
				return fmt.Errorf("marshal site create payload: %w", err)
			}
			cl := safelinecmd.NewClient()
			env, err := cl.Do("POST", ctx.Endpoint, bytes.NewReader(body), nil)
			if err != nil {
				var recovered bool
				env, warnings, recovered, err = recoverCreateFileStorageError(cl, ctx.Endpoint, payload, env, err, warnings)
				if !recovered {
					return err
				}
			}
			return safelinecmd.PrintResult(c, map[string]any{"ok": true, "operation": "site.create", "warnings": warnings, "errors": []string{}, "data": map[string]any{"endpoint": ctx.Endpoint, "response": env.Data, "rollback": "safeline site delete <id> --yes"}})
		},
	}
	c.AddCommand(newCreateCapabilitiesCmd())
	c.Flags().StringVar(&opts.Name, "name", "", "Site name")
	c.Flags().StringArrayVar(&opts.Domains, "domain", nil, "Site domain, repeatable; '*' is accepted")
	c.Flags().IntSliceVar(&opts.Ports, "port", nil, "Listener port, repeatable")
	c.Flags().BoolVar(&opts.SSL, "ssl", false, "Enable HTTPS on the listener")
	c.Flags().IntVar(&opts.CertID, "cert-id", 0, "SSL certificate ID required with --ssl")
	c.Flags().BoolVar(&opts.HTTP2, "http2", false, "Enable HTTP/2; requires --ssl")
	c.Flags().BoolVar(&opts.SNI, "sni", false, "Enable SNI; requires --ssl")
	c.Flags().BoolVar(&opts.NonHTTP, "non-http", false, "Create non-HTTP listener")
	c.Flags().StringVar(&opts.URLPath, "url-path", "/", "URL path match value")
	c.Flags().StringVar(&opts.URLPathOp, "url-path-op", "pre", "URL path operation, such as pre")
	c.Flags().StringVar(&opts.BackendType, "backend-type", "proxy", "Backend type: proxy or redirect")
	c.Flags().StringArrayVar(&opts.Upstreams, "upstream", nil, "Proxy upstream URL, repeatable")
	c.Flags().StringVar(&opts.LoadBalance, "load-balance", "Round Robin", "Proxy load balance method")
	c.Flags().StringVar(&opts.XFFAction, "xff-action", "append", "X-Forwarded-For action")
	c.Flags().StringVar(&opts.RedirectURL, "redirect-url", "", "Redirect target URL")
	c.Flags().IntVar(&opts.RedirectCode, "redirect-code", 302, "Redirect status code: 301, 302, 307, or 308")
	c.Flags().IntVar(&opts.PolicyGroupID, "policy-group", 0, "Policy group ID")
	c.Flags().BoolVar(&opts.Enable, "enable", false, "Create site enabled instead of disabled")
	c.Flags().StringVar(&request, "request", "", "Raw JSON request body")
	c.Flags().BoolVar(&opts.Check, "check", false, "Validate and print request without writing")
	c.Flags().BoolVar(&opts.Explain, "explain", false, "Print generated request without writing")
	c.Flags().BoolVar(&opts.Yes, "yes", false, "Confirm write operation")
	return c
}

func newCreateCapabilitiesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "capabilities",
		Short: "Show site create capabilities for the current SafeLine target",
		RunE: func(c *cobra.Command, args []string) error {
			ctx, err := safelineruntime.ResolveContext(safelinecmd.NewClient(), safelineruntime.Options{
				VersionOverride:       safelinecmd.VersionOverride,
				OperationModeOverride: safelinecmd.OperationModeOverride,
				ConfigVersion:         safelinecmd.ConfigVersion,
				ConfigOperationMode:   safelinecmd.ConfigOperationMode,
			})
			if err != nil {
				return err
			}
			caps := safelineruntime.SiteCreateCapabilities(ctx)
			return safelinecmd.PrintResult(c, map[string]any{
				"ok":        true,
				"operation": "site.create.capabilities",
				"warnings":  ctx.Warnings,
				"errors":    []string{},
				"data":      map[string]any{"context": ctx, "capabilities": caps},
			})
		},
	}
}
