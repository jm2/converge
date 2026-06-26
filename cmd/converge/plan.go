package main

import (
	"github.com/TsekNet/converge/internal/engine"
	"github.com/TsekNet/converge/internal/exit"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan [blueprint|manifest.hcl]",
	Short: "Show what would change without making changes",
	Long:  "Run all resource checks and display a diff of pending changes. Accepts a blueprint name or a path to an .hcl manifest. Does not require root.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		printer := makePrinter()
		printer.Banner(app.Version())
		printer.BlueprintHeader(args[0])

		app.EngineOpts.Timeout = timeout

		// HCL manifest path: build the graph from the manifest and run the same
		// engine plan the blueprint path uses. Blueprint names are unaffected.
		if isManifestPath(args[0]) {
			g, err := loadManifestGraph(args[0])
			if err != nil {
				exitWithError(exit.Error, err)
			}
			code, err := engine.RunPlanDAG(g, printer, app.EngineOpts)
			if err != nil {
				exitWithError(code, err)
			}
			exitWithCode(code)
			return
		}

		code, err := app.RunPlan(args[0], printer)
		if err != nil {
			exitWithError(code, err)
		}
		exitWithCode(code)
	},
}

func init() {
	rootCmd.AddCommand(planCmd)
}
