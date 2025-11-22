package main

import (
	"fmt"
	"net"
	"os"

	"github.com/spf13/cobra"
	"tcbroker/pkg/config"
)

var (
	checkInterfaces bool
)

var validateCmd = &cobra.Command{
	Use:   "validate [config-file]",
	Short: "Validates a configuration file without applying it.",
	Long: `Reads and validates the given YAML configuration file, checking for
syntax errors, logical inconsistencies, and optionally verifying that
specified network interfaces exist on the system.`,
	Args: cobra.ExactArgs(1),
	Run:  validate,
}

func init() {
	rootCmd.AddCommand(validateCmd)
	validateCmd.Flags().BoolVar(&checkInterfaces, "check-interfaces", false, "Verify that specified network interfaces exist on the system")
}

func validate(cmd *cobra.Command, args []string) {
	configFile := args[0]

	fmt.Printf("Validating configuration file: %s\n", configFile)

	// Load and validate the configuration
	cfg, err := config.Load(configFile)
	if err != nil {
		fmt.Printf("❌ Validation failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Configuration syntax is valid\n")
	fmt.Printf("✓ Found %d rule(s)\n", len(cfg.Rules))

	// Display configuration summary
	for i, rule := range cfg.Rules {
		fmt.Printf("\n  Rule #%d (%s):\n", i+1, rule.Name)
		fmt.Printf("    Source Interface: %s\n", rule.SrcIntf)
		fmt.Printf("    Destination Interface: %s\n", rule.DstIntf)
		fmt.Printf("    Action: mirror\n")
		if rule.Rewrite != nil {
			fmt.Printf("    Rewrite:\n")
			if rule.Rewrite.DstMAC != "" {
				fmt.Printf("      Dst MAC: %s\n", rule.Rewrite.DstMAC)
			}
			if rule.Rewrite.SrcMAC != "" {
				fmt.Printf("      Src MAC: %s\n", rule.Rewrite.SrcMAC)
			}
			if rule.Rewrite.DstIP != "" {
				fmt.Printf("      Dst IP: %s\n", rule.Rewrite.DstIP)
			}
			if rule.Rewrite.SrcIP != "" {
				fmt.Printf("      Src IP: %s\n", rule.Rewrite.SrcIP)
			}
		}
		fmt.Printf("    Filters: %d\n", len(rule.Filters))
	}

	// Optionally check if interfaces exist on the system
	if checkInterfaces {
		fmt.Printf("\nChecking network interfaces...\n")
		allInterfacesExist := true

		// Collect unique interfaces
		interfaceSet := make(map[string]bool)
		for _, rule := range cfg.Rules {
			interfaceSet[rule.SrcIntf] = true
			interfaceSet[rule.DstIntf] = true
		}

		// Check each unique interface
		for ifaceName := range interfaceSet {
			if _, errIface := net.InterfaceByName(ifaceName); errIface != nil {
				fmt.Printf("  ❌ Interface '%s' not found: %v\n", ifaceName, errIface)
				allInterfacesExist = false
			} else {
				fmt.Printf("  ✓ Interface '%s' exists\n", ifaceName)
			}
		}

		if !allInterfacesExist {
			fmt.Printf("\n❌ Some network interfaces do not exist on this system.\n")
			os.Exit(1)
		}
	}

	fmt.Printf("\n✓ Configuration is valid and ready to use!\n")
}
