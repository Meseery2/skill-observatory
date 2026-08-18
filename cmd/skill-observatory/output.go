package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/meseery/skill-observatory/internal/clierr"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func outputFormat() string {
	f := strings.ToLower(viper.GetString("format"))
	if f == "" {
		return "table"
	}
	return f
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeTable(w io.Writer, headers []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, strings.Join(headers, "\t")); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(tw, strings.Join(row, "\t")); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func writeErrln(cmd *cobra.Command, a ...any) error {
	_, err := fmt.Fprintln(cmd.ErrOrStderr(), a...)
	return err
}

func writeErrf(cmd *cobra.Command, format string, a ...any) error {
	_, err := fmt.Fprintf(cmd.ErrOrStderr(), format, a...)
	return err
}

func writeOutput(cmd *cobra.Command, v any, headers []string, rows [][]string) error {
	switch outputFormat() {
	case "json":
		return writeJSON(cmd.OutOrStdout(), v)
	case "table":
		return writeTable(cmd.OutOrStdout(), headers, rows)
	default:
		return clierr.Usage(fmt.Errorf("unknown format %q (want table or json)", outputFormat()))
	}
}
