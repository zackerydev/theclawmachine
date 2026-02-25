package service

import (
	"strings"
	"testing"

	"helm.sh/helm/v4/pkg/chart/common"
	commonutil "helm.sh/helm/v4/pkg/chart/common/util"
	"helm.sh/helm/v4/pkg/engine"
)

func renderBotTemplate(t *testing.T, botType BotType, values map[string]any, templateSuffix string) string {
	t.Helper()

	chrt, err := loadBotChart(botType)
	if err != nil {
		t.Fatalf("loadBotChart(%s): %v", botType, err)
	}

	renderValues, err := commonutil.ToRenderValues(chrt, values, common.ReleaseOptions{
		Name:      "test-bot",
		Namespace: "claw-machine",
		Revision:  1,
		IsInstall: true,
	}, common.DefaultCapabilities)
	if err != nil {
		t.Fatalf("ToRenderValues: %v", err)
	}

	rendered, err := engine.Render(chrt, renderValues)
	if err != nil {
		t.Fatalf("engine.Render: %v", err)
	}

	for name, manifest := range rendered {
		if strings.HasSuffix(name, templateSuffix) {
			return manifest
		}
	}

	t.Fatalf("template %q not rendered", templateSuffix)
	return ""
}

func renderBotTemplateErr(t *testing.T, botType BotType, values map[string]any, templateSuffix string) (string, error) {
	t.Helper()

	chrt, err := loadBotChart(botType)
	if err != nil {
		t.Fatalf("loadBotChart(%s): %v", botType, err)
	}

	renderValues, err := commonutil.ToRenderValues(chrt, values, common.ReleaseOptions{
		Name:      "test-bot",
		Namespace: "claw-machine",
		Revision:  1,
		IsInstall: true,
	}, common.DefaultCapabilities)
	if err != nil {
		t.Fatalf("ToRenderValues: %v", err)
	}

	rendered, err := engine.Render(chrt, renderValues)
	if err != nil {
		return "", err
	}

	for name, manifest := range rendered {
		if strings.HasSuffix(name, templateSuffix) {
			return manifest, nil
		}
	}

	t.Fatalf("template %q not rendered", templateSuffix)
	return "", nil
}

func TestOpenClawBackupRestoreInitContainerRequiresRestoreToggle(t *testing.T) {
	values := map[string]any{
		"backup": map[string]any{
			"enabled":          true,
			"restoreOnStartup": false,
			"s3": map[string]any{
				"bucket": "workspace-backups",
			},
		},
	}
	manifest := renderBotTemplate(t, BotTypeOpenClaw, values, "templates/deployment.yaml")
	if strings.Contains(manifest, "name: restore-workspace") {
		t.Fatal("restore init container rendered when restoreOnStartup=false")
	}

	values = map[string]any{
		"backup": map[string]any{
			"enabled":          true,
			"restoreOnStartup": true,
			"s3": map[string]any{
				"bucket": "workspace-backups",
			},
			"credentials": map[string]any{
				"accessKeyIdSecretRef": map[string]any{
					"name": "backup-access-target",
					"key":  "value",
				},
				"secretAccessKeySecretRef": map[string]any{
					"name": "backup-secret-target",
					"key":  "value",
				},
			},
		},
	}
	manifest = renderBotTemplate(t, BotTypeOpenClaw, values, "templates/deployment.yaml")
	if !strings.Contains(manifest, "name: restore-workspace") {
		t.Fatal("restore init container missing when restoreOnStartup=true")
	}
	if !strings.Contains(manifest, "name: AWS_ACCESS_KEY_ID") {
		t.Fatal("restore init container missing AWS_ACCESS_KEY_ID env var")
	}
	if !strings.Contains(manifest, "name: AWS_SECRET_ACCESS_KEY") {
		t.Fatal("restore init container missing AWS_SECRET_ACCESS_KEY env var")
	}
}

func TestOpenClawStartupScriptUsesCLIReconciliation(t *testing.T) {
	manifest := renderBotTemplate(t, BotTypeOpenClaw, map[string]any{
		"agent": map[string]any{
			"defaultModel": "anthropic/claude-haiku-4.5",
		},
	}, "templates/startup-configmap.yaml")

	if !strings.Contains(manifest, "openclaw onboard") {
		t.Fatal("startup script must reconcile provider auth via openclaw onboard")
	}
	if !strings.Contains(manifest, "openclaw config set") {
		t.Fatal("startup script must reconcile channel/gateway values via openclaw config set")
	}
	if !strings.Contains(manifest, "openclaw models set") {
		t.Fatal("startup script missing default model reconciliation via openclaw models set")
	}
	if !strings.Contains(manifest, `CONFIG_DIR="${OPENCLAW_HOME:-${HOME:-/root}/.openclaw}"`) {
		t.Fatal("startup script must derive config dir from root HOME with OPENCLAW_HOME override")
	}
	if !strings.Contains(manifest, "channels.telegram.botToken") {
		t.Fatal("startup script must set telegram bot token using channels.telegram.botToken")
	}
	if strings.Contains(manifest, "channels.telegram.token") {
		t.Fatal("startup script must not use deprecated channels.telegram.token path")
	}
	if strings.Contains(manifest, "No config found, running openclaw setup...") {
		t.Fatal("startup script should not run legacy openclaw setup bootstrap")
	}
	if strings.Contains(manifest, "--skip-skills") {
		t.Fatal("startup script should not pass --skip-skills to openclaw onboard")
	}
	if strings.Contains(manifest, "--skip-channels") {
		t.Fatal("startup script should not pass --skip-channels to openclaw onboard")
	}
	if !strings.Contains(manifest, "--skip-health") {
		t.Fatal("startup script should pass --skip-health to openclaw onboard")
	}
	if strings.Contains(manifest, "--skip-ui") {
		t.Fatal("startup script should not pass --skip-ui to openclaw onboard")
	}
	if !strings.Contains(manifest, `plugins.entries.discord.enabled`) {
		t.Fatal("startup script should reconcile plugins.entries.discord.enabled")
	}
	if !strings.Contains(manifest, `plugins.entries.telegram.enabled`) {
		t.Fatal("startup script should reconcile plugins.entries.telegram.enabled")
	}
	if !strings.Contains(manifest, `plugins.entries.slack.enabled`) {
		t.Fatal("startup script should reconcile plugins.entries.slack.enabled")
	}
	if strings.Contains(manifest, "node -e") {
		t.Fatal("startup script should not mutate OpenClaw config with inline node scripts")
	}
}

func TestOpenClawDeploymentDoesNotRenderConfigSeedInitContainer(t *testing.T) {
	manifest := renderBotTemplate(t, BotTypeOpenClaw, map[string]any{
		"backup": map[string]any{
			"enabled":          false,
			"restoreOnStartup": false,
		},
	}, "templates/deployment.yaml")

	if strings.Contains(manifest, "name: config-init") {
		t.Fatal("openclaw deployment should not render config-init")
	}
	if strings.Contains(manifest, "name: config-seed") {
		t.Fatal("openclaw deployment should not render config-seed volume")
	}
}

func TestOpenClawBackupCronJobCredentialWiring(t *testing.T) {
	values := map[string]any{
		"backup": map[string]any{
			"enabled": true,
			"s3": map[string]any{
				"bucket": "workspace-backups",
			},
			"credentials": map[string]any{
				"accessKeyIdSecretRef": map[string]any{
					"name": "backup-access-target",
					"key":  "value",
				},
				"secretAccessKeySecretRef": map[string]any{
					"name": "backup-secret-target",
					"key":  "value",
				},
			},
		},
	}
	manifest := renderBotTemplate(t, BotTypeOpenClaw, values, "templates/backup-cronjob.yaml")
	if !strings.Contains(manifest, "name: AWS_ACCESS_KEY_ID") {
		t.Fatal("backup cronjob missing AWS_ACCESS_KEY_ID env var")
	}
	if !strings.Contains(manifest, "name: backup-access-target") {
		t.Fatal("backup cronjob missing access key secret reference name")
	}
	if !strings.Contains(manifest, "name: AWS_SECRET_ACCESS_KEY") {
		t.Fatal("backup cronjob missing AWS_SECRET_ACCESS_KEY env var")
	}
	if strings.Contains(manifest, "envFrom:") {
		t.Fatal("backup cronjob should not use envFrom when credential secret refs are set")
	}

	values = map[string]any{
		"backup": map[string]any{
			"enabled":           true,
			"credentialsSecret": "legacy-backup-secret",
			"s3": map[string]any{
				"bucket": "workspace-backups",
			},
		},
	}
	manifest = renderBotTemplate(t, BotTypeOpenClaw, values, "templates/backup-cronjob.yaml")
	if !strings.Contains(manifest, "envFrom:") {
		t.Fatal("backup cronjob missing legacy envFrom secretRef")
	}
	if !strings.Contains(manifest, "name: legacy-backup-secret") {
		t.Fatal("backup cronjob missing legacy credentials secret name")
	}
}

func TestIronClawDeployment_MigrationsDisabledByDefault(t *testing.T) {
	values := map[string]any{
		"postgresql": map[string]any{
			"password": "test-password",
		},
	}

	manifest := renderBotTemplate(t, BotTypeIronClaw, values, "templates/deployment.yaml")
	if strings.Contains(manifest, "name: migrate") {
		t.Fatal("migrate init container should not render by default")
	}
}

func TestIronClawSecretsRequirePostgresPasswordWhenEnabled(t *testing.T) {
	_, err := renderBotTemplateErr(t, BotTypeIronClaw, map[string]any{}, "templates/secret.yaml")
	if err == nil {
		t.Fatal("expected render error when postgresql.enabled=true and password empty")
	}
	if !strings.Contains(err.Error(), "postgresql.password must be set") {
		t.Fatalf("unexpected render error: %v", err)
	}
}

func TestIronClawDatabaseURLContainsPostgresPassword(t *testing.T) {
	values := map[string]any{
		"postgresql": map[string]any{
			"password": "test-password",
		},
	}

	manifest := renderBotTemplate(t, BotTypeIronClaw, values, "templates/secret.yaml")
	if !strings.Contains(manifest, "postgres://ironclaw:test-password@") {
		t.Fatalf("expected DATABASE_URL with password, got:\n%s", manifest)
	}
}

func TestIronClawStartupScriptUsesTCPProbeAndDeploymentHasStartupProbe(t *testing.T) {
	startupManifest := renderBotTemplate(t, BotTypeIronClaw, map[string]any{
		"postgresql": map[string]any{
			"password": "test-password",
		},
	}, "templates/startup-configmap.yaml")

	if !strings.Contains(startupManifest, "nc -z -w 1") {
		t.Fatal("expected startup script to use nc TCP check for PostgreSQL")
	}
	if strings.Contains(startupManifest, "wget -qO /dev/null") {
		t.Fatal("startup script should not use wget HTTP check for PostgreSQL")
	}
	if !strings.Contains(startupManifest, "--no-onboard") {
		t.Fatal("expected startup script to run ironclaw with --no-onboard when supported")
	}

	deploymentManifest := renderBotTemplate(t, BotTypeIronClaw, map[string]any{
		"postgresql": map[string]any{
			"password": "test-password",
		},
	}, "templates/deployment.yaml")

	if !strings.Contains(deploymentManifest, "startupProbe:") {
		t.Fatal("expected startupProbe in ironclaw deployment")
	}
}

func TestIronClawNetworkPolicyAllowsPostgresIngress(t *testing.T) {
	manifest := renderBotTemplate(t, BotTypeIronClaw, map[string]any{
		"networkPolicy": map[string]any{
			"ingress": true,
		},
		"postgresql": map[string]any{
			"enabled":  true,
			"password": "test-password",
		},
	}, "templates/networkpolicy.yaml")

	if !strings.Contains(manifest, "name: test-bot-ironclaw-allow-postgres") {
		t.Fatal("expected allow-postgres network policy for ironclaw")
	}
	if !strings.Contains(manifest, "port: 5432") {
		t.Fatal("expected postgres network policy to allow port 5432")
	}
}

func TestBotNetworkPolicyEgressToggleRendersAllowEgressPolicy(t *testing.T) {
	tests := []struct {
		name    string
		botType BotType
		values  map[string]any
	}{
		{
			name:    "openclaw",
			botType: BotTypeOpenClaw,
			values: map[string]any{
				"networkPolicy": map[string]any{"egress": false},
			},
		},
		{
			name:    "picoclaw",
			botType: BotTypePicoClaw,
			values: map[string]any{
				"networkPolicy": map[string]any{"egress": false},
			},
		},
		{
			name:    "ironclaw",
			botType: BotTypeIronClaw,
			values: map[string]any{
				"networkPolicy": map[string]any{"egress": false},
				"postgresql": map[string]any{
					"enabled":  true,
					"password": "test-password",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manifest := renderBotTemplate(t, tc.botType, tc.values, "templates/networkpolicy.yaml")
			if !strings.Contains(manifest, "name: test-bot-"+tc.name+"-allow-egress") {
				t.Fatalf("expected allow-egress network policy for %s", tc.name)
			}
			if !strings.Contains(manifest, "egress: []") {
				t.Fatalf("expected allow-egress policy to deny all egress for %s", tc.name)
			}
			if strings.Contains(manifest, "name: test-bot-"+tc.name+"-allow-egress") && strings.Contains(manifest, "egress:\n    - {}") {
				t.Fatalf("allow-egress policy should not allow all egress for %s", tc.name)
			}
		})
	}
}

func TestPicoClawBackupRestoreInitContainerRequiresRestoreToggle(t *testing.T) {
	values := map[string]any{
		"backup": map[string]any{
			"enabled":          true,
			"restoreOnStartup": false,
			"s3": map[string]any{
				"bucket": "workspace-backups",
			},
		},
	}
	manifest := renderBotTemplate(t, BotTypePicoClaw, values, "templates/deployment.yaml")
	if strings.Contains(manifest, "name: restore-workspace") {
		t.Fatal("picoclaw restore init container rendered when restoreOnStartup=false")
	}

	values = map[string]any{
		"backup": map[string]any{
			"enabled":          true,
			"restoreOnStartup": true,
			"s3": map[string]any{
				"bucket": "workspace-backups",
			},
		},
	}
	manifest = renderBotTemplate(t, BotTypePicoClaw, values, "templates/deployment.yaml")
	if !strings.Contains(manifest, "name: restore-workspace") {
		t.Fatal("picoclaw restore init container missing when restoreOnStartup=true")
	}
}
