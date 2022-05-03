/*
Copyright © 2022 Merbridge Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/merbridge/merbridge/config"
	"github.com/merbridge/merbridge/controller"
	cniserver "github.com/merbridge/merbridge/internal/cni-server"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "mbctl",
	Short: "Use eBPF to speed up your Service Mesh like crossing an Einstein-Rosen Bridge.",
	Long:  `Use eBPF to speed up your Service Mesh like crossing an Einstein-Rosen Bridge.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if config.EnableCNI {
			s := cniserver.NewServer("/host/var/run/merbridge-cni.sock", "/sys/fs/bpf")
			if err := s.Start(); err != nil {
				log.Fatal(err)
				return err
			}
			installCNI(cmd.Context())
		}
		// todo wait for stop
		if err := controller.Run(); err != nil {
			log.Fatal(err)
			return err
		}
		return nil
	},
}

// Execute excute root command and its child commands
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Setup log format
	log.SetFormatter(&log.TextFormatter{
		DisableTimestamp:       false,
		FullTimestamp:          true,
		DisableLevelTruncation: true,
		DisableColors:          true,
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			fs := strings.Split(f.File, "/")
			filename := fs[len(fs)-1]
			ff := strings.Split(f.Function, "/")
			_f := ff[len(ff)-1]
			return fmt.Sprintf("%s()", _f), fmt.Sprintf("%s:%d", filename, f.Line)
		},
	})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)
	log.SetReportCaller(true)

	// Get some flags from commands
	rootCmd.PersistentFlags().StringVarP(&config.Mode, "mode", "m", config.ModeIstio, "Service mesh mode, current support istio and linkerd")
	rootCmd.PersistentFlags().BoolVarP(&config.UseReconnect, "use-reconnect", "r", true, "Use re-connect mode as same-node acceleration")
	rootCmd.PersistentFlags().BoolVarP(&config.Debug, "debug", "d", false, "Debug mode")
	rootCmd.PersistentFlags().BoolVarP(&config.IsKind, "kind", "k", false, "Kubernetes in Kind mode")
	rootCmd.PersistentFlags().BoolVar(&config.EnableCNI, "cni-mode", false, "Enable Merbridge CNI plugin")
	rootCmd.PersistentFlags().StringVarP(&config.IpsFile, "ips-file", "f", "", "Current node ips file name")
}

func installCNI(ctx context.Context) {
	installer := cniserver.NewInstaller()
	go func() {
		if err := installer.Run(ctx); err != nil {
			log.Error(err)
		}
		if err := installer.Cleanup(); err != nil {
			log.Errorf("Failed to clean up Merbridge CNI: %v", err)
		}
	}()

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGINT)
		<-ch
		if err := installer.Cleanup(); err != nil {
			log.Errorf("Failed to clean up Merbridge CNI: %v", err)
		}
	}()
}
