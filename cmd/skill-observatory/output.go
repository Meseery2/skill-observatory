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
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	return tw.Flush()
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
