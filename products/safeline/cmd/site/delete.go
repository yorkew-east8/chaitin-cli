package site

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	safelinecmd "github.com/chaitin/chaitin-cli/products/safeline/cmd"
	safelineruntime "github.com/chaitin/chaitin-cli/products/safeline/runtime"
	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	var yes bool
	var check bool
	c := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a SafeLine site by numeric ID",
		Args:  cobra.ExactArgs(1),
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
				return fmt.Errorf("site delete is unsupported for operation mode %q", ctx.OperationMode)
			}
			payload, err := buildDeletePayload(args[0])
			if err != nil {
				return err
			}
			ids := payload["id__in"].([]int)
			preview := map[string]any{"ok": true, "operation": "site.delete.check", "warnings": ctx.Warnings, "errors": []string{}, "data": map[string]any{"endpoint": ctx.Endpoint, "payload": payload}}
			if check || safelinecmd.IsDryRun() {
				return safelinecmd.PrintResult(c, preview)
			}
			if !yes {
				return fmt.Errorf("site delete is a write operation; re-run with --yes or use --check to preview")
			}
			body, err := json.Marshal(payload)
			if err != nil {
				return fmt.Errorf("marshal site delete payload: %w", err)
			}
			cl := safelinecmd.NewClient()
			env, err := cl.Do("DELETE", ctx.Endpoint, bytes.NewReader(body), nil)
			if err != nil {
				var recovered bool
				env, ctx.Warnings, recovered, err = recoverDeleteFileStorageError(cl, ctx.Endpoint, ids[0], env, err, ctx.Warnings)
				if !recovered {
					return err
				}
			}
			return safelinecmd.PrintResult(c, map[string]any{"ok": true, "operation": "site.delete", "warnings": ctx.Warnings, "errors": []string{}, "data": map[string]any{"endpoint": ctx.Endpoint, "response": env.Data}})
		},
	}
	c.Flags().BoolVar(&yes, "yes", false, "Confirm delete operation")
	c.Flags().BoolVar(&check, "check", false, "Print delete request without writing")
	return c
}

func buildDeletePayload(rawID string) (map[string]any, error) {
	id, err := strconv.Atoi(rawID)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("site id must be a positive integer")
	}
	return map[string]any{"id__in": []int{id}}, nil
}
