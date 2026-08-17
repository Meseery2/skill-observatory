package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/meseery/skill-observatory/internal/fsutil"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill-observatory",
		Short: "Measure whether installed agent skills fire and whether they help",
		Long: `Skill Observatory inventories SKILL.md files, mines Cursor transcripts
for real invocations, and runs trigger plus with/without quality evals.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig(cmd)
		},
	}

	home, _ := os.UserHomeDir()
	defaultDB := filepath.Join(home, ".local", "share", "skill-observatory", "observatory.db")
	defaultTranscripts := filepath.Join(home, ".cursor", "projects")
	defaultConfigDir := filepath.Join(home, ".config", "skill-observatory")

	cmd.PersistentFlags().String("config", "", "config file (default "+defaultConfigDir+"/config.yaml)")
	cmd.PersistentFlags().String("format", "table", "output format: table or json")
	cmd.PersistentFlags().String("db", defaultDB, "sqlite database path")
	cmd.PersistentFlags().String("model", "", "llm model (overrides llm.model)")
	cmd.PersistentFlags().String("provider", "", "llm provider: openai, anthropic, openai-compatible")
	cmd.PersistentFlags().String("transcripts-root", defaultTranscripts, "Cursor projects root containing agent-transcripts")
	cmd.PersistentFlags().String("evals-dir", "evals", "directory of per-skill eval fixtures")
	cmd.PersistentFlags().StringSlice("project", nil, "extra project roots to scan for skills")
	cmd.PersistentFlags().String("log-level", "info", "log level (debug, info, warn, error)")

	_ = viper.BindPFlag("format", cmd.PersistentFlags().Lookup("format"))
	_ = viper.BindPFlag("db", cmd.PersistentFlags().Lookup("db"))
	_ = viper.BindPFlag("model", cmd.PersistentFlags().Lookup("model"))
	_ = viper.BindPFlag("provider", cmd.PersistentFlags().Lookup("provider"))
	_ = viper.BindPFlag("transcripts_root", cmd.PersistentFlags().Lookup("transcripts-root"))
	_ = viper.BindPFlag("evals_dir", cmd.PersistentFlags().Lookup("evals-dir"))
	_ = viper.BindPFlag("project", cmd.PersistentFlags().Lookup("project"))
	_ = viper.BindPFlag("log-level", cmd.PersistentFlags().Lookup("log-level"))

	cmd.AddCommand(
		newDiscoverCmd(),
		newInventoryCmd(),
		newLogCmd(),
		newEvalCmd(),
		newReportCmd(),
		newVersionCmd(),
	)
	return cmd
}

func initConfig(cmd *cobra.Command) error {
	cfgFile, _ := cmd.Flags().GetString("config")
	if cfgFile != "" {
		viper.SetConfigFile(fsutil.ExpandHome(cfgFile))
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("finding home directory: %w", err)
		}
		viper.AddConfigPath(filepath.Join(home, ".config", "skill-observatory"))
		viper.AddConfigPath(".")
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	viper.SetEnvPrefix("SKILL_OBSERVATORY")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("reading config: %w", err)
		}
	}

	level := slog.LevelInfo
	switch strings.ToLower(viper.GetString("log-level")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: level})))
	return nil
}
