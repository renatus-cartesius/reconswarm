/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"time"

	"reconswarm/internal/config"
	"reconswarm/internal/control"
	"reconswarm/internal/logging"
	"reconswarm/internal/control/provisioning"
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

		// Create SSH key provider using etcd endpoints from config
		keyProvider := ssh.NewKeyProvider(cfg.Etcd.Endpoints)
		defer keyProvider.Close()

		// Get or create SSH key pair from etcd
		logging.Logger().Info("Getting SSH key pair from etcd")
		keyPair, err := keyProvider.GetOrCreate(ctx)
		if err != nil {
			logging.Logger().Fatal("Failed to get SSH key pair", zap.Error(err))
		}

		logging.Logger().Info("SSH key pair ready")

		// Create provisioner using factory
		logging.Logger().Info("Creating provisioner", zap.String("type", string(cfg.Provisioner.Type)))
		provisioner, err := provisioning.NewProvisioner(cfg.Provisioner)
		if err != nil {
			logging.Logger().Fatal("Failed to create provisioner", zap.Error(err))
		}

		// Get VM defaults from provisioner config
		vmDefaults := provisioning.GetVMDefaults(cfg.Provisioner)

		// Get VM name
		name := fmt.Sprintf("reconswarm-demo-%v", time.Now().Unix())

		// Create VM specification
		spec := provisioning.InstanceSpec{
			Name:         name,
			Cores:        vmDefaults.Cores,
			Memory:       vmDefaults.Memory,
			DiskSize:     vmDefaults.DiskSize,
			ImageID:      vmDefaults.Image,
			Zone:         vmDefaults.Zone,
			SSHPublicKey: keyPair.PublicKey,
			Username:     vmDefaults.Username,
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
				zap.Int("setup_commands", len(cfg.Workers.SetupCommands)))

			// Create control configuration - use private key from etcd
			controlConfig := control.Config{
				Host:         instance.IP,
				User:         vmDefaults.Username,
				PrivateKey:   keyPair.PrivateKey, // Use key content from etcd
				Timeout:      5 * time.Minute,
				SSHTimeout:   30 * time.Second,
				InstanceName: instance.Name,
			}

			// Create controller
			logging.Logger().Debug("Creating SSH controller")
			controller, err := control.NewController(controlConfig)
			if err != nil {
				logging.Logger().Warn("Failed to create controller", zap.Error(err))
			} else {
				defer func() {
					if err := controller.Close(); err != nil {
						logging.Logger().Warn("failed to close controller",
							zap.String("instance", instance.IP),
							zap.Error(err))
					}
				}()

				// Run setup commands from configuration
				logging.Logger().Info("Starting VM setup",
					zap.String("ip", instance.IP),
					zap.Int("command_count", len(cfg.Workers.SetupCommands)))

				for i, cmd := range cfg.Workers.SetupCommands {
					logging.Logger().Debug("Executing setup command",
						zap.Int("step", i+1),
						zap.Int("total", len(cfg.Workers.SetupCommands)),
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
}
