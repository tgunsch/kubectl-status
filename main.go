package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

type options struct {
	selection     string
	namespace     string
	allNamespaces bool
}

func parseArgs(args []string, stderr io.Writer) (options, error) {
	var opts options
	flags := flag.NewFlagSet("kubectl status", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.namespace, "n", "", "Namespace")
	flags.StringVar(&opts.namespace, "namespace", "", "Namespace")
	flags.BoolVar(&opts.allNamespaces, "A", false, "All namespaces")
	flags.BoolVar(&opts.allNamespaces, "all-namespaces", false, "All namespaces")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: kubectl status [-n NAMESPACE | -A] TYPE[/NAME] | TYPE[,TYPE...]")
		fmt.Fprintln(stderr, "Shows conditions for selected resources (newest transition first).")
		flags.PrintDefaults()
	}

	// Go's flag parser stops at the first positional argument, while kubectl
	// allows flags on either side of the resource selection.
	ordered := make([]string, 0, len(args))
	positionals := make([]string, 0, 1)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-n" || arg == "--namespace":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s requires a namespace", arg)
			}
			ordered = append(ordered, arg, args[i+1])
			i++
		case arg == "-A" || arg == "--all-namespaces" || arg == "-h" || arg == "--help":
			ordered = append(ordered, arg)
		case strings.HasPrefix(arg, "-n=") || strings.HasPrefix(arg, "--namespace="):
			ordered = append(ordered, arg)
		case strings.HasPrefix(arg, "-"):
			return opts, fmt.Errorf("unsupported flag %q", arg)
		default:
			positionals = append(positionals, arg)
		}
	}
	if err := flags.Parse(append(ordered, positionals...)); err != nil {
		return opts, err
	}
	if opts.namespace != "" && opts.allNamespaces {
		return opts, errors.New("-n and -A cannot be used together")
	}
	if flags.NArg() == 0 {
		flags.Usage()
		return opts, errors.New("expected a resource type, type list, or type/name")
	}
	if flags.NArg() != 1 {
		return opts, errors.New("expected one resource type, type list, or type/name")
	}
	opts.selection = flags.Arg(0)
	if err := validateSelection(opts.selection); err != nil {
		return opts, err
	}
	return opts, nil
}

func validateSelection(selection string) error {
	if selection == "" || strings.HasPrefix(selection, "-") {
		return errors.New("invalid resource selection")
	}
	for _, part := range strings.Split(selection, ",") {
		if part == "" || strings.ContainsAny(part, " \t\n\r") || strings.HasPrefix(part, "-") {
			return errors.New("invalid resource selection")
		}
		if strings.Count(part, "/") > 1 || (strings.Contains(part, "/") && (strings.Contains(selection, ",") || strings.HasSuffix(part, "/") || strings.HasPrefix(part, "/"))) {
			return errors.New("expected one type/name or a comma-separated list of types")
		}
	}
	return nil
}

type resource struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Status struct {
		Conditions []condition `json:"conditions"`
	} `json:"status"`
}

type condition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason"`
	Message            string `json:"message"`
	LastTransitionTime string `json:"lastTransitionTime"`
}

func decodeResources(data []byte) ([]resource, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil // kubectl --ignore-not-found may produce no JSON.
	}
	var envelope struct {
		Kind  string     `json:"kind"`
		Items []resource `json:"items"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("decode kubectl output: %w", err)
	}
	if strings.HasSuffix(envelope.Kind, "List") && envelope.Kind != "" {
		return envelope.Items, nil
	}
	var single resource
	if err := json.Unmarshal(data, &single); err != nil {
		return nil, fmt.Errorf("decode resource: %w", err)
	}
	if single.Kind == "" || single.Metadata.Name == "" {
		return nil, errors.New("kubectl returned a resource without a kind or name")
	}
	return []resource{single}, nil
}

func fetch(opts options) ([]resource, error) {
	args := []string{"get", opts.selection, "-o", "json", "--ignore-not-found"}
	if opts.namespace != "" {
		args = append(args, "-n", opts.namespace)
	}
	if opts.allNamespaces {
		args = append(args, "-A")
	}
	cmd := exec.Command("kubectl", args...)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			message := strings.TrimSpace(string(exitErr.Stderr))
			if message != "" {
				return nil, errors.New(message)
			}
		}
		return nil, fmt.Errorf("kubectl get: %w", err)
	}
	return decodeResources(output)
}

func display(value string) string {
	if value == "" {
		return "-"
	}
	return strings.NewReplacer("\r", `\r`, "\n", `\n`, "\t", `\t`).Replace(value)
}

func transitionTime(c condition) time.Time {
	t, err := time.Parse(time.RFC3339Nano, c.LastTransitionTime)
	if err != nil {
		return time.Time{}
	}
	return t
}

func render(w io.Writer, resources []resource) error {
	if len(resources) == 0 {
		_, err := fmt.Fprintln(w, "No resources found")
		return err
	}
	for i, r := range resources {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		heading := strings.ToLower(r.Kind) + "/" + r.Metadata.Name
		if r.Metadata.Namespace != "" {
			heading = r.Metadata.Namespace + "/" + heading
		}
		if _, err := fmt.Fprintln(w, heading); err != nil {
			return err
		}
		if len(r.Status.Conditions) == 0 {
			if _, err := fmt.Fprintln(w, "No conditions"); err != nil {
				return err
			}
			continue
		}
		conditions := append([]condition(nil), r.Status.Conditions...)
		sort.SliceStable(conditions, func(i, j int) bool {
			return transitionTime(conditions[i]).Before(transitionTime(conditions[j]))
		})
		table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		if _, err := fmt.Fprintln(table, "TYPE\tSTATUS\tREASON\tMESSAGE\tLAST TRANSITION TIME"); err != nil {
			return err
		}
		for _, c := range conditions {
			if _, err := fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", display(c.Type), display(c.Status), display(c.Reason), display(c.Message), display(c.LastTransitionTime)); err != nil {
				return err
			}
		}
		if err := table.Flush(); err != nil {
			return err
		}
	}
	return nil
}

func run(args []string, stdout, stderr io.Writer) int {
	opts, err := parseArgs(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(stderr, "kubectl status:", err)
		return 1
	}
	resources, err := fetch(opts)
	if err == nil {
		err = render(stdout, resources)
	}
	if err != nil {
		fmt.Fprintln(stderr, "kubectl status:", err)
		return 1
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
