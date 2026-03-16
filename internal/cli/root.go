package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"bbbserver-cli/internal/api"
	"bbbserver-cli/internal/config"
	"bbbserver-cli/internal/output"
	"bbbserver-cli/internal/postman"
	"bbbserver-cli/internal/version"

	"github.com/spf13/cobra"
)

type App struct {
	root *cobra.Command

	flagBaseURL  string
	flagAPIKey   string
	flagTimeout  time.Duration
	flagOutput   string
	flagPretty   bool
	flagVerbose  bool
	flagDebug    bool
	flagConfig   string
	flagAuthMode string

	cfg        config.Settings
	endpoints  []postman.Endpoint
	parseError error
}

func New() *App {
	app := &App{}
	app.root = app.newRootCommand()
	return app
}

func (a *App) Execute() int {
	err := a.root.Execute()
	if err == nil {
		return api.ExitCodeOK
	}

	renderer := output.New(a.effectiveOutputMode(), a.effectivePretty())
	_ = renderer.Error(err)
	return api.ExitCode(err)
}

func (a *App) newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "bbbserver-cli",
		Short:         "CLI for the bbbserver SaaS HTTP API",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return a.loadConfig(cmd)
		},
	}

	cmd.PersistentFlags().StringVar(&a.flagBaseURL, "base-url", "", "API base URL")
	cmd.PersistentFlags().StringVar(&a.flagAPIKey, "api-key", "", "API key")
	cmd.PersistentFlags().DurationVar(&a.flagTimeout, "timeout", 30*time.Second, "Request timeout")
	cmd.PersistentFlags().StringVar(&a.flagOutput, "output", "human", "Output format: human|json")
	cmd.PersistentFlags().BoolVar(&a.flagPretty, "pretty", false, "Pretty JSON output (json mode only)")
	cmd.PersistentFlags().BoolVar(&a.flagVerbose, "verbose", false, "Verbose output")
	cmd.PersistentFlags().BoolVar(&a.flagDebug, "debug", false, "Enable sanitized HTTP debug output")
	cmd.PersistentFlags().StringVar(&a.flagAuthMode, "auth-mode", "", "Auth mode: apikey|bearer")
	cmd.PersistentFlags().StringVar(&a.flagConfig, "config", "", "Config file path")
	_ = cmd.PersistentFlags().MarkHidden("config")
	_ = cmd.PersistentFlags().MarkHidden("auth-mode")

	cmd.AddCommand(a.newVersionCommand())
	cmd.AddCommand(a.newHealthCommand())
	cmd.AddCommand(a.newMeCommand())
	cmd.AddCommand(a.newListCommand())
	cmd.AddCommand(a.newCompletionCommand(cmd))
	cmd.AddCommand(a.newConfigCommand())

	a.endpoints, a.parseError = a.loadEndpoints()
	if a.parseError == nil {
		a.addDynamicEndpointCommands(cmd)
	}

	return cmd
}

func (a *App) loadEndpoints() ([]postman.Endpoint, error) {
	endpoints, err := postman.ParseEmbedded()
	if err != nil {
		return nil, fmt.Errorf("load embedded collection: %w", err)
	}
	return endpoints, nil
}

func (a *App) loadConfig(cmd *cobra.Command) error {
	options := config.LoadOptions{
		ConfigPath: a.flagConfig,
		BaseURL:    changedString(cmd, "base-url", a.flagBaseURL),
		APIKey:     changedString(cmd, "api-key", a.flagAPIKey),
		Timeout:    changedDuration(cmd, "timeout", a.flagTimeout),
		Output:     changedString(cmd, "output", a.flagOutput),
		Pretty:     changedBool(cmd, "pretty", a.flagPretty),
		Verbose:    changedBool(cmd, "verbose", a.flagVerbose),
		Debug:      changedBool(cmd, "debug", a.flagDebug),
		AuthMode:   changedString(cmd, "auth-mode", a.flagAuthMode),
	}

	cfg, err := config.Load(options)
	if err != nil {
		return err
	}

	if cfg.Output != "human" && cfg.Output != "json" {
		return &api.ValidationError{Message: "--output must be one of: human, json"}
	}

	a.cfg = cfg
	return nil
}

func (a *App) newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print CLI version",
		RunE: func(_ *cobra.Command, _ []string) error {
			return a.renderer().Data(map[string]any{"version": version.Version})
		},
	}
}

func (a *App) newHealthCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check API reachability",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(a.cfg.BaseURL) == "" {
				return &api.ValidationError{Message: "base URL is required (set with --base-url, env, or config)"}
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), a.cfg.Timeout)
			defer cancel()

			client := a.apiClient()
			resp, err := client.Do(ctx, api.Request{Method: http.MethodGet, Path: "/", RequireAuth: false})
			if err != nil {
				var apiErr *api.APIError
				if errors.As(err, &apiErr) {
					result := map[string]any{
						"reachable":      true,
						"status":         apiErr.Status,
						"auth_required":  apiErr.Status == 401 || apiErr.Status == 403,
						"status_message": apiErr.Message,
						"ok":             false,
					}

					if apiErr.Status == 401 || apiErr.Status == 403 {
						apiKeyConfigured := strings.TrimSpace(a.cfg.APIKey) != ""
						result["auth_required"] = true
						result["api_key_configured"] = apiKeyConfigured
						if apiKeyConfigured {
							authResp, authErr := client.Do(ctx, api.Request{Method: http.MethodGet, Path: "/", RequireAuth: true})
							if authErr == nil {
								result["status"] = authResp.Status
								result["status_message"] = ""
								result["ok"] = true
							} else {
								var authAPIErr *api.APIError
								if errors.As(authErr, &authAPIErr) {
									result["status"] = authAPIErr.Status
									result["status_message"] = authAPIErr.Message
								} else {
									return authErr
								}
							}
						}
					}

					return a.renderHealth(cmd, result)
				}
				return err
			}

			return a.renderHealth(cmd, map[string]any{
				"reachable": true,
				"status":    resp.Status,
				"auth_required": false,
				"ok":        true,
			})
		},
	}
}

func (a *App) renderHealth(cmd *cobra.Command, data map[string]any) error {
	if a.cfg.Output == "json" {
		return a.renderer().Data(data)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Reachable: %s\n", yesNo(data["reachable"]))
	if status, ok := data["status"]; ok {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "HTTP Status: %v\n", status)
	}
	if authRequired, ok := data["auth_required"]; ok {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Auth Required: %s\n", yesNo(authRequired))
	}
	if okField, ok := data["ok"]; ok {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "OK: %s\n", yesNo(okField))
	}
	if message, ok := data["status_message"]; ok {
		messageText := strings.TrimSpace(fmt.Sprintf("%v", message))
		if messageText != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Message: %s\n", messageText)
		}
	}
	if configured, ok := data["api_key_configured"]; ok {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "API Key Configured: %s\n", yesNo(configured))
	}

	return nil
}

func yesNo(value any) string {
	boolean, ok := value.(bool)
	if !ok {
		return "no"
	}
	if boolean {
		return "yes"
	}
	return "no"
}

func (a *App) newMeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Show info about the authenticated user",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(a.cfg.APIKey) == "" {
				return &api.ValidationError{Message: "api key is required (set with --api-key, env, or config)"}
			}
			if strings.TrimSpace(a.cfg.BaseURL) == "" {
				return &api.ValidationError{Message: "base URL is required (set with --base-url, env, or config)"}
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), a.cfg.Timeout)
			defer cancel()

			client := a.apiClient()
			resp, err := client.Do(ctx, api.Request{Method: http.MethodGet, Path: "/", RequireAuth: true})
			if err != nil {
				return err
			}
			return a.renderer().Data(resp.Body)
		},
	}
}

func (a *App) newListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List collection-driven commands",
		RunE: func(_ *cobra.Command, _ []string) error {
			if a.parseError != nil {
				return fmt.Errorf("load embedded collection: %w", a.parseError)
			}

			rows := make([]map[string]any, 0, len(a.endpoints))
			for _, endpoint := range a.endpoints {
				commandPath := make([]string, 0, len(endpoint.Groups)+2)
				for _, group := range endpoint.Groups {
					commandPath = append(commandPath, postman.Slug(group))
				}
				commandPath = append(commandPath, postman.Slug(endpoint.Name))

				rows = append(rows, map[string]any{
					"command": strings.Join(commandPath, " "),
					"method":  endpoint.Method,
					"path":    endpoint.Path,
				})
			}
			return a.renderer().Data(rows)
		},
	}
}

func (a *App) newCompletionCommand(root *cobra.Command) *cobra.Command {
	completionCmd := &cobra.Command{
		Use:   "completion",
		Short: "Generate shell completion scripts",
	}

	completionCmd.AddCommand(&cobra.Command{
		Use:   "bash",
		Short: "Generate bash completion",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return root.GenBashCompletion(cmd.OutOrStdout())
		},
	})

	completionCmd.AddCommand(&cobra.Command{
		Use:   "zsh",
		Short: "Generate zsh completion",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return root.GenZshCompletion(cmd.OutOrStdout())
		},
	})

	completionCmd.AddCommand(&cobra.Command{
		Use:   "fish",
		Short: "Generate fish completion",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return root.GenFishCompletion(cmd.OutOrStdout(), true)
		},
	})

	completionCmd.AddCommand(&cobra.Command{
		Use:   "powershell",
		Short: "Generate PowerShell completion",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return root.GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
		},
	})

	return completionCmd
}

func (a *App) newConfigCommand() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
	}

	configCmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Initialize config file",
		RunE: func(_ *cobra.Command, _ []string) error {
			path, err := config.Init(a.flagConfig)
			if err != nil {
				return err
			}
			return a.renderer().Data(map[string]any{"config_path": path, "initialized": true})
		},
	})

	configCmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show effective config (masked)",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load(config.LoadOptions{ConfigPath: a.flagConfig})
			if err != nil {
				return err
			}
			path := a.flagConfig
			if path == "" {
				path, err = config.DefaultConfigPath()
				if err != nil {
					return err
				}
			}
			return a.renderer().Data(config.MaskedMap(cfg, path))
		},
	})

	setCmd := &cobra.Command{Use: "set", Short: "Set config values"}

	setCmd.AddCommand(&cobra.Command{
		Use:   "base-url <url>",
		Short: "Set base URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			path, err := config.SetValue(a.flagConfig, "base_url", args[0])
			if err != nil {
				return err
			}
			return a.renderer().Data(map[string]any{"config_path": path, "updated": "base_url"})
		},
	})

	setCmd.AddCommand(&cobra.Command{
		Use:   "api-key <key>",
		Short: "Set API key",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			path, err := config.SetValue(a.flagConfig, "api_key", args[0])
			if err != nil {
				return err
			}
			return a.renderer().Data(map[string]any{"config_path": path, "updated": "api_key"})
		},
	})

	configCmd.AddCommand(setCmd)
	return configCmd
}

func (a *App) addDynamicEndpointCommands(root *cobra.Command) {
	groupIndex := map[string]*cobra.Command{}

	for _, endpoint := range a.endpoints {
		parent := root
		groupKey := ""
		for _, group := range endpoint.Groups {
			slug := postman.Slug(group)
			groupKey += "/" + slug
			next, ok := groupIndex[groupKey]
			if !ok {
				next = &cobra.Command{
					Use:   slug,
					Short: group,
				}
				parent.AddCommand(next)
				groupIndex[groupKey] = next
			}
			parent = next
		}

		ep := endpoint
		leaf := &cobra.Command{
			Use:   postman.Slug(ep.Name),
			Short: fmt.Sprintf("%s %s", ep.Method, ep.Path),
			Long:  endpointLongDescription(ep),
			Example: endpointExamples(ep),
			RunE: func(cmd *cobra.Command, _ []string) error {
				if strings.TrimSpace(a.cfg.APIKey) == "" {
					return &api.ValidationError{Message: "api key is required (set with --api-key, env, or config)"}
				}
				if strings.TrimSpace(a.cfg.BaseURL) == "" {
					return &api.ValidationError{Message: "base URL is required (set with --base-url, env, or config)"}
				}

				queryValues := map[string]string{}
				for _, key := range ep.QueryParams {
					flag := postman.FlagName(key)
					if cmd.Flags().Changed(flag) {
						value, _ := cmd.Flags().GetString(flag)
						queryValues[key] = value
					}
				}

				path := ep.Path
				for _, key := range ep.PathParams {
					flag := postman.FlagName(key)
					value, _ := cmd.Flags().GetString(flag)
					if strings.TrimSpace(value) == "" {
						return &api.ValidationError{Message: fmt.Sprintf("missing required path parameter --%s", flag)}
					}
					path = strings.ReplaceAll(path, "{"+key+"}", value)
					path = strings.ReplaceAll(path, ":"+key, value)
					path = strings.ReplaceAll(path, "{{"+key+"}}", value)
				}

				ctx, cancel := context.WithTimeout(cmd.Context(), a.cfg.Timeout)
				defer cancel()

				if ep.HasFileFields() {
					fields := make([]api.MultipartField, 0)
					for _, bf := range ep.BodyFields {
						flag := postman.FlagName(bf.Key)
						if !cmd.Flags().Changed(flag) {
							continue
						}
						value, _ := cmd.Flags().GetString(flag)
						if bf.IsFile {
							fields = append(fields, api.MultipartField{Key: bf.Key, FilePath: value})
						} else {
							fields = append(fields, api.MultipartField{Key: bf.Key, Value: value})
						}
					}
					response, err := a.apiClient().DoMultipart(ctx, ep.Method, path, queryValues, fields)
					if err != nil {
						return err
					}
					return a.renderer().Data(response.Body)
				}

				var body []byte
				if ep.HasBody {
					dataArg, _ := cmd.Flags().GetString("data")
					parsedBody, err := parseDataArg(dataArg)
					if err != nil {
						return err
					}
					body = parsedBody
				}

				response, err := a.apiClient().Do(ctx, api.Request{
					Method:      ep.Method,
					Path:        path,
					Query:       queryValues,
					Body:        body,
					ContentType: "application/json",
					RequireAuth: true,
				})
				if err != nil {
					return err
				}

				return a.renderer().Data(response.Body)
			},
		}

		for _, key := range ep.QueryParams {
			leaf.Flags().String(postman.FlagName(key), "", fmt.Sprintf("Query parameter %s", key))
		}
		for _, key := range ep.PathParams {
			leaf.Flags().String(postman.FlagName(key), "", fmt.Sprintf("Path parameter %s", key))
		}
		if ep.HasFileFields() {
			for _, bf := range ep.BodyFields {
				if bf.IsFile {
					leaf.Flags().String(postman.FlagName(bf.Key), "", fmt.Sprintf("File path for %s", bf.Key))
				} else {
					leaf.Flags().String(postman.FlagName(bf.Key), "", fmt.Sprintf("Form field %s", bf.Key))
				}
			}
		} else if ep.HasBody {
			leaf.Flags().String("data", "", `JSON body, inline or @file.json`)
		}

		parent.AddCommand(leaf)
	}

	orderCommands(root)
}

func endpointLongDescription(ep postman.Endpoint) string {
	base := strings.TrimSpace(ep.Description)
	if base == "" {
		base = fmt.Sprintf("%s %s", ep.Method, ep.Path)
	}

	if !ep.HasBody || len(ep.BodyFields) == 0 {
		return base
	}

	var b strings.Builder
	b.WriteString(base)
	b.WriteString("\n\nBody fields (from Postman):")
	for _, field := range ep.BodyFields {
		b.WriteString("\n- ")
		b.WriteString(field.Key)
	}

	return b.String()
}

func endpointExamples(ep postman.Endpoint) string {
	command := "bbbserver-cli " + strings.Join(commandSegments(ep), " ")
	examples := make([]string, 0)

	if len(ep.QueryParams) > 0 {
		parts := []string{command}
		for _, query := range ep.QueryParams {
			parts = append(parts, fmt.Sprintf("--%s <value>", postman.FlagName(query)))
		}
		examples = append(examples, strings.Join(parts, " "))
	}

	if ep.HasBody {
		inline := command + " --data '" + bodyJSONTemplate(ep.BodyFields, 5) + "'"
		examples = append(examples, inline)
		examples = append(examples, command+" --data @payload.json")
	}

	if len(examples) == 0 {
		examples = append(examples, command)
	}

	return strings.Join(examples, "\n")
}

func commandSegments(ep postman.Endpoint) []string {
	parts := make([]string, 0, len(ep.Groups)+1)
	for _, group := range ep.Groups {
		parts = append(parts, postman.Slug(group))
	}
	parts = append(parts, postman.Slug(ep.Name))
	return parts
}

func bodyJSONTemplate(fields []postman.BodyField, maxFields int) string {
	if len(fields) == 0 {
		return "{}"
	}
	selected := selectExampleFields(fields, maxFields)
	if len(selected) == 0 {
		selected = fields
	}

	var b bytes.Buffer
	b.WriteString("{")
	for index, field := range selected {
		if index > 0 {
			b.WriteString(",")
		}

		keyJSON, _ := json.Marshal(field.Key)
		value := exampleValueForField(field)
		valueJSON, _ := json.Marshal(value)

		b.WriteString(string(keyJSON))
		b.WriteString(":")
		b.WriteString(string(valueJSON))
	}
	b.WriteString("}")

	return b.String()
}

func selectExampleFields(fields []postman.BodyField, maxFields int) []postman.BodyField {
	if maxFields <= 0 || len(fields) <= maxFields {
		return fields
	}

	priority := []string{"roomId", "conferenceId", "name", "startTime", "duration", "maxConnections"}
	result := make([]postman.BodyField, 0, maxFields)
	used := map[string]struct{}{}

	for _, key := range priority {
		for _, field := range fields {
			if len(result) >= maxFields {
				return result
			}
			if field.Key != key {
				continue
			}
			result = append(result, field)
			used[field.Key] = struct{}{}
		}
	}

	for _, field := range fields {
		if len(result) >= maxFields {
			break
		}
		if _, ok := used[field.Key]; ok {
			continue
		}
		if strings.Contains(field.SampleValue, "{") && strings.Contains(field.SampleValue, "}") {
			continue
		}
		if len(field.SampleValue) > 80 {
			continue
		}
		result = append(result, field)
	}

	return result
}

func exampleValueForField(field postman.BodyField) string {
	key := strings.ToLower(strings.TrimSpace(field.Key))
	switch key {
	case "roomid":
		return "room-id"
	case "conferenceid":
		return "conference-id"
	case "name":
		return "Conference"
	case "starttime":
		return "YYYY-MM-DD hh:mm:ss"
	case "duration":
		return "60"
	case "maxconnections":
		return "5"
	default:
		value := strings.TrimSpace(field.SampleValue)
		if value == "" || len(value) > 40 || strings.Contains(value, "{") {
			return "value"
		}
		return value
	}
}

func (a *App) apiClient() *api.Client {
	return &api.Client{
		BaseURL:   a.cfg.BaseURL,
		APIKey:    a.cfg.APIKey,
		AuthMode:  a.cfg.AuthMode,
		HTTP:      api.NewDefaultHTTPClient(a.cfg.Timeout),
		UserAgent: fmt.Sprintf("bbbserver-cli/%s", version.Version),
		Debug:     a.cfg.Debug,
	}
}

func (a *App) renderer() output.Renderer {
	return output.New(a.cfg.Output, a.cfg.Pretty)
}

func (a *App) effectiveOutputMode() string {
	if a.cfg.Output != "" {
		return a.cfg.Output
	}
	if a.flagOutput != "" {
		return a.flagOutput
	}
	return "human"
}

func (a *App) effectivePretty() bool {
	if a.cfg.Output != "" {
		return a.cfg.Pretty
	}
	return a.flagPretty
}

func changedString(cmd *cobra.Command, flagName, value string) *string {
	if cmd.Flags().Changed(flagName) {
		v := value
		return &v
	}
	return nil
}

func changedDuration(cmd *cobra.Command, flagName string, value time.Duration) *time.Duration {
	if cmd.Flags().Changed(flagName) {
		v := value
		return &v
	}
	return nil
}

func changedBool(cmd *cobra.Command, flagName string, value bool) *bool {
	if cmd.Flags().Changed(flagName) {
		v := value
		return &v
	}
	return nil
}

func parseDataArg(value string) ([]byte, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}

	var payload []byte
	if strings.HasPrefix(trimmed, "@") {
		path := strings.TrimSpace(strings.TrimPrefix(trimmed, "@"))
		if path == "" {
			return nil, &api.ValidationError{Message: "--data @file requires a file path"}
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, &api.ValidationError{Message: fmt.Sprintf("read data file: %v", err)}
		}
		payload = content
	} else {
		payload = []byte(trimmed)
	}

	var parsed any
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return nil, &api.ValidationError{Message: fmt.Sprintf("--data must be valid JSON: %v", err)}
	}

	return payload, nil
}

func orderCommands(root *cobra.Command) {
	for _, command := range root.Commands() {
		if len(command.Commands()) > 0 {
			children := command.Commands()
			sort.Slice(children, func(i, j int) bool {
				return children[i].Use < children[j].Use
			})
			orderCommands(command)
		}
	}
}
