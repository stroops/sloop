/*
Copyright © 2026 tuanngo

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/stroops/sloop/internal/cli/commands"
	_ "github.com/stroops/sloop/internal/cli/commands"
)

var (
	cfgFile string
	version string
	commit  string
	date    string
)

// SetVersion được gọi từ main.go (ldflags)
func SetVersion(v, c, d string) {
	version, commit, date = v, c, d
}

var rootCmd = &cobra.Command{
	Use:   "sloop",
	Short: "Sloop - AI workspace wrapper for terminal dev tools",
	Long: `⚓ sloop - Navigate your AI fleet.

A terminal workspace manager for persistent, context-aware AI sessions.
Create, detach, attach, and navigate between AI workspaces without losing flow.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	Version:       version,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: $HOME/.sloop/config.yaml)")
	rootCmd.PersistentFlags().String("socket", filepath.Join(os.TempDir(), "sloop.sock"), "daemon socket path")
	rootCmd.PersistentFlags().Bool("no-color", false, "disable colored output")

	viper.BindPFlag("socket", rootCmd.PersistentFlags().Lookup("socket"))
	viper.BindPFlag("no_color", rootCmd.PersistentFlags().Lookup("no-color"))

	// Auto-register commands
	commands.Register(rootCmd)
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		configDir := filepath.Join(home, ".sloop")
		viper.AddConfigPath(configDir)
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")

		// Tạo ~/.sloop nếu chưa có
		if _, err := os.Stat(configDir); os.IsNotExist(err) {
			os.MkdirAll(configDir, 0755)
		}
	}

	viper.SetEnvPrefix("SLOOP")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}
