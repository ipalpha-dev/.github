package cli

import (
	"os"
	"path/filepath"

	"github.com/ipalpha-dev/tooling/internal/env"
	"github.com/ipalpha-dev/tooling/internal/envfile"
	"github.com/ipalpha-dev/tooling/internal/gateway"
	"github.com/ipalpha-dev/tooling/internal/i18n"
	"github.com/ipalpha-dev/tooling/internal/ui"
	"github.com/spf13/cobra"
)

func gatewayCmd() *cobra.Command {
	var mode, base string
	cmd := &cobra.Command{Use: "gateway", Short: i18n.T("gateway_cmd"), Long: i18n.T("gateway_long"), Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := open("ai gateway")
			if err != nil {
				return err
			}
			if mode == "" {
				if !ui.Interactive() {
					return ui.NewProblem(i18n.T("gateway_cmd"), i18n.T("gateway_need_mode"), "ipalpha ai gateway --mode simulator")
				}
				mode, err = ui.Select(i18n.T("gateway_choose"), i18n.T("gateway_description"), []ui.Option{{Value: "simulator", Label: i18n.T("gateway_simulator"), Hint: i18n.T("gateway_simulator_hint")}, {Value: "real", Label: i18n.T("gateway_real"), Hint: i18n.T("gateway_real_hint")}}, w.Settings.Gateway())
				if err != nil {
					return err
				}
			}
			if mode != "simulator" && mode != "real" {
				return ui.NewProblem(i18n.T("gateway_cmd"), i18n.T("gateway_invalid"))
			}
			clientKey, managementKey := "", ""
			if mode == "real" {
				current := envfile.Read(filepath.Join(w.Repo("ai-api"), ".env"))
				if base == "" {
					base, err = ui.Input(i18n.T("gateway_url"), i18n.T("gateway_url_hint"), current["AI_BASE_URL"], func(v string) error {
						_, err := gateway.NormalizeURL(v)
						if err != nil {
							return ui.NewProblem(i18n.T("gateway_url"), i18n.T("gateway_url_invalid"))
						}
						return nil
					})
					if err != nil {
						return err
					}
				}
				clientKey = os.Getenv("CPAMC_SETUP_CLIENT_KEY")
				managementKey = os.Getenv("CPAMC_SETUP_MANAGEMENT_KEY")
				if ui.Interactive() {
					if clientKey == "" {
						clientKey, err = ui.Secret(i18n.T("gateway_client_key"), i18n.T("gateway_client_hint"))
						if err != nil {
							return err
						}
					}
					if managementKey == "" {
						managementKey, err = ui.Secret(i18n.T("gateway_management_key"), i18n.T("gateway_management_hint"))
						if err != nil {
							return err
						}
					}
				}
			}
			// Create the API env from the current templates first; setup never removes other keys.
			if _, err := env.Prepare(w); err != nil {
				return err
			}
			if err := gateway.Configure(w, mode, base, clientKey, managementKey); err != nil {
				return ui.NewProblem(i18n.T("gateway_cmd"), i18n.T("gateway_invalid"), i18n.T("gateway_keys_hint"))
			}
			ui.Done(i18n.T("gateway_saved", mode))
			return nil
		}}
	cmd.Flags().StringVar(&mode, "mode", "", i18n.T("gateway_mode_flag"))
	cmd.Flags().StringVar(&base, "url", "", i18n.T("gateway_url_flag"))
	return cmd
}
