/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"reconswarm/internal/config"
	"reconswarm/internal/control"
	"reconswarm/internal/logging"
	"reconswarm/internal/provisioning"
	"reconswarm/internal/ssh"

	"go.uber.org/zap"

	"github.com/spf13/cobra"
)

// debugCmd represents the debug command
var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Debug command to create and setup VM for testing",
	Long: `Debug command creates a VM instance in Yandex Cloud and sets it up using SSH.
This is useful for testing the provisioning and automation functionality.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Load configuration
		logging.Logger().Info("Loading configuration")
		cfg, err := config.Load()
		if err != nil {
			logging.Logger().Fatal("Failed to load configuration", zap.Error(err))
		}

		ctx := context.Background()

		// Get or generate SSH key pair
		logging.Logger().Info("Getting or generating SSH key pair")
		keyDir := "keys"
		keyPair, err := ssh.GetOrGenerateKeyPair(keyDir)
		if err != nil {
			logging.Logger().Fatal("Failed to get or generate SSH key pair", zap.Error(err))
		}

		logging.Logger().Info("Using SSH key pair",
			zap.String("private_key", keyPair.PrivateKeyPath),
			zap.String("public_key", keyPair.PublicKeyPath))

		// Create provisioner (SDK is created inside)
		logging.Logger().Info("Creating Yandex Cloud provisioner")
		provisioner, err := provisioning.NewYcProvisioner(cfg.IAMToken, cfg.FolderID)
		if err != nil {
			logging.Logger().Fatal("Failed to create provisioner", zap.Error(err))
		}

		// Get VM parameters from environment variables or use defaults
		name := os.Getenv("YC_INSTANCE_NAME")
		if name == "" {
			name = fmt.Sprintf("reconswarm-demo-%v", time.Now().Unix())
		}

		// Create VM specification
		spec := provisioning.InstanceSpec{
			Name:         name,
			Cores:        cfg.DefaultCores,
			Memory:       cfg.DefaultMemory,
			DiskSize:     cfg.DefaultDiskSize,
			ImageID:      cfg.DefaultImage,
			Zone:         cfg.DefaultZone,
			SSHPublicKey: keyPair.PublicKey,
			Username:     cfg.DefaultUsername,
		}

		// Create VM
		logging.Logger().Info("Creating VM",
			zap.String("name", spec.Name),
			zap.String("zone", spec.Zone),
			zap.Int("cores", int(spec.Cores)),
			zap.Int64("memory_gb", spec.Memory))

		instance, err := provisioner.Create(ctx, spec)
		if err != nil {
			logging.Logger().Fatal("Failed to create VM", zap.Error(err))
		}

		logging.Logger().Info("VM created successfully",
			zap.String("id", instance.ID),
			zap.String("name", instance.Name),
			zap.String("ip", instance.IP),
			zap.String("zone", instance.Zone),
			zap.String("status", instance.Status))

		// Setup VM using SSH (if IP is available)
		if instance.IP != "" {
			logging.Logger().Info("Starting VM setup via SSH",
				zap.String("ip", instance.IP),
				zap.Int("setup_commands", len(cfg.SetupCommands)))

			// Create control configuration
			controlConfig := control.Config{
				Host:       instance.IP,
				User:       cfg.DefaultUsername,
				PrivateKey: keyPair.PrivateKeyPath,
				Timeout:    5 * time.Minute,
				SSHTimeout: 30 * time.Second,
			}

			// Create controller
			logging.Logger().Debug("Creating SSH controller")
			controller, err := control.NewController(controlConfig)
			if err != nil {
				logging.Logger().Warn("Failed to create controller", zap.Error(err))
			} else {
				defer controller.Close()

				// Run setup commands from configuration
				logging.Logger().Info("Starting VM setup",
					zap.String("ip", instance.IP),
					zap.Int("command_count", len(cfg.SetupCommands)))

				for i, cmd := range cfg.SetupCommands {
					logging.Logger().Debug("Executing setup command",
						zap.Int("step", i+1),
						zap.Int("total", len(cfg.SetupCommands)),
						zap.String("command", cmd))

					err = controller.Run(cmd)
					if err != nil {
						logging.Logger().Error("Failed to execute setup command",
							zap.String("command", cmd),
							zap.Int("step", i+1),
							zap.Error(err))
						logging.Logger().Warn("Failed to setup VM using SSH", zap.Error(err))
						break
					}
				}

				if err == nil {
					logging.Logger().Info("VM setup completed successfully")
					fmt.Printf("VM setup completed successfully using SSH\n")
				}
			}
		} else {
			logging.Logger().Warn("IP address not obtained, skipping SSH setup")
		}

		// Delete VM
		// fmt.Printf("\nDeleting VM '%s'...\n", instance.ID)
		// err = provisioner.Delete(ctx, instance.ID)
		// if err != nil {
		// 	log.Fatalf("Failed to delete VM: %v", err)
		// }
		//
		// fmt.Printf("VM deleted successfully\n")
	},
}

func init() {
	rootCmd.AddCommand(debugCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// debugCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// debugCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
