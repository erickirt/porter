package porter

import (
	"context"
	"testing"
	"time"

	"get.porter.sh/porter/pkg/cnab"
	"get.porter.sh/porter/pkg/printer"
	"get.porter.sh/porter/pkg/secrets"
	"get.porter.sh/porter/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"reflect"
)

func TestNewDisplayInstallation(t *testing.T) {
	t.Run("installation has been installed", func(t *testing.T) {
		cp := storage.NewTestInstallationProvider(t)
		defer cp.Close()

		install := cp.CreateInstallation(storage.NewInstallation("", "wordpress"), func(i *storage.Installation) {
			i.Status.Action = cnab.ActionUpgrade
			i.Status.ResultStatus = cnab.StatusRunning
		})

		_, err := cp.GetInstallation(context.Background(), "", "wordpress")
		require.NoError(t, err, "ReadInstallation failed")

		displayInstall := NewDisplayInstallation(install)

		require.Equal(t, displayInstall.Name, install.Name, "invalid installation name")
		require.Equal(t, displayInstall.Status.Created, install.Status.Created, "invalid created time")
		require.Equal(t, displayInstall.Status.Modified, install.Status.Modified, "invalid modified time")
		require.Equal(t, cnab.ActionUpgrade, displayInstall.Status.Action, "invalid last action")
		require.Equal(t, cnab.StatusRunning, displayInstall.Status.ResultStatus, "invalid last status")
	})

	t.Run("installation has not been installed", func(t *testing.T) {
		cp := storage.NewTestInstallationProvider(t)
		defer cp.Close()

		install := cp.CreateInstallation(storage.NewInstallation("", "wordpress"))

		_, err := cp.GetInstallation(context.Background(), "", "wordpress")
		require.NoError(t, err, "GetInst failed")

		displayInstall := NewDisplayInstallation(install)

		require.Equal(t, displayInstall.Name, install.Name, "invalid installation name")
		require.Equal(t, install.Status.Created, displayInstall.Status.Created, "invalid created time")
		require.Equal(t, install.Status.Modified, displayInstall.Status.Modified, "invalid modified time")
		require.Empty(t, displayInstall.Status.Action, "invalid last action")
		require.Empty(t, displayInstall.Status.ResultStatus, "invalid last status")
	})
}

func TestPorter_ListInstallations(t *testing.T) {
	ctx := context.Background()
	p := NewTestPorter(t)
	defer p.Close()

	i1 := storage.NewInstallation("", "shared-mysql")
	i1.Parameters.Parameters = []secrets.SourceMap{ // Define a parameter that is stored in a secret, list should not retrieve it
		{Name: "password", Source: secrets.Source{Strategy: "secret", Hint: "mypassword"}},
	}
	i1.Status.RunID = "10" // Add a run but don't populate the data for it, list should not retrieve it

	p.TestInstallations.CreateInstallation(i1)
	p.TestInstallations.CreateInstallation(storage.NewInstallation("dev", "carolyn-wordpress"))
	p.TestInstallations.CreateInstallation(storage.NewInstallation("dev", "vaughn-wordpress"))
	p.TestInstallations.CreateInstallation(storage.NewInstallation("test", "staging-wordpress"))
	p.TestInstallations.CreateInstallation(storage.NewInstallation("test", "iat-wordpress"))
	p.TestInstallations.CreateInstallation(storage.NewInstallation("test", "shared-mysql"))

	t.Run("all-namespaces", func(t *testing.T) {
		opts := ListOptions{AllNamespaces: true}
		results, err := p.ListInstallations(ctx, opts)
		require.NoError(t, err)
		assert.Len(t, results, 6)

		// Check that porter didn't go off and retrieve extra data for each installation
		for _, r := range results {
			assert.Empty(t, r.ResolvedParameters, "ListInstallations should not resolve secrets used by the installations")
		}
	})

	t.Run("local namespace", func(t *testing.T) {
		opts := ListOptions{Namespace: "dev"}
		results, err := p.ListInstallations(ctx, opts)
		require.NoError(t, err)
		assert.Len(t, results, 2)

		opts = ListOptions{Namespace: "test"}
		results, err = p.ListInstallations(ctx, opts)
		require.NoError(t, err)
		assert.Len(t, results, 3)
	})

	t.Run("global namespace", func(t *testing.T) {
		opts := ListOptions{Namespace: ""}
		results, err := p.ListInstallations(ctx, opts)
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})

	t.Run("empty namespace", func(t *testing.T) {
		opts := ListOptions{Namespace: "nonexistent"}
		results, err := p.ListInstallations(ctx, opts)
		require.NoError(t, err)
		assert.Len(t, results, 0)
		assert.NotNil(t, results)
	})
}

func TestPorter_ListInstallationsWithFieldSelector(t *testing.T) {
	ctx := context.Background()
	p := NewTestPorter(t)
	defer p.Close()

	i1 := storage.NewInstallation("", "installation1")
	i1.Bundle.Version = "0.2.0"
	i1.Status.Action = "install"
	i1.Status.ResultStatus = "succeeded"

	i2 := storage.NewInstallation("", "installation2")
	i2.Bundle.Version = "0.2.1"
	i2.Status.Action = "install"
	i2.Status.ResultStatus = "succeeded"

	i3 := storage.NewInstallation("", "installation3")
	i3.Bundle.Version = "0.2.0"
	i3.Status.Action = "install"
	i3.Status.ResultStatus = "failed"

	i4 := storage.NewInstallation("", "installation4")
	i4.Bundle.Version = "0.2.5"
	i4.Status.Action = "install"

	i5 := storage.NewInstallation("", "installation5")
	i5.Bundle.Version = "0.1.0"
	i5.Status.Action = "install"
	i5.Status.ResultStatus = "succeeded"

	p.TestInstallations.CreateInstallation(i1)
	p.TestInstallations.CreateInstallation(i2)
	p.TestInstallations.CreateInstallation(i3)
	p.TestInstallations.CreateInstallation(i4)
	p.TestInstallations.CreateInstallation(i5)

	t.Run("blank field selector", func(t *testing.T) {
		opts := ListOptions{FieldSelector: ""}
		results, err := p.ListInstallations(ctx, opts)
		require.NoError(t, err)
		assert.Len(t, results, 5)
	})

	t.Run("top level field selectors", func(t *testing.T) {
		opts := ListOptions{FieldSelector: "name=installation1"}
		results, err := p.ListInstallations(ctx, opts)
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})

	t.Run("multi level field selectors", func(t *testing.T) {
		opts := ListOptions{FieldSelector: "name=installation1,bundle.version=0.2.0,status.action=install,status.resultStatus=succeeded"}
		results, err := p.ListInstallations(ctx, opts)
		require.NoError(t, err)
		assert.Len(t, results, 1)

		opts = ListOptions{FieldSelector: "status.action=install,status.resultStatus=succeeded"}
		results, err = p.ListInstallations(ctx, opts)
		require.NoError(t, err)
		assert.Len(t, results, 3)

		opts = ListOptions{FieldSelector: "status.resultStatus=failed"}
		results, err = p.ListInstallations(ctx, opts)
		require.NoError(t, err)
		assert.Len(t, results, 1)

		opts = ListOptions{FieldSelector: "status.resultStatus"}
		results, err = p.ListInstallations(ctx, opts)
		require.Error(t, err)
		assert.Len(t, results, 0)

		opts = ListOptions{FieldSelector: "status.resultStatus=xyz"}
		results, err = p.ListInstallations(ctx, opts)
		require.NoError(t, err)
		assert.Len(t, results, 0)

	})
}

func TestDisplayInstallation_ConvertToInstallation(t *testing.T) {
	cp := storage.NewTestInstallationProvider(t)
	defer cp.Close()

	install := cp.CreateInstallation(storage.NewInstallation("", "wordpress"), func(i *storage.Installation) {
		i.Status.Action = cnab.ActionUpgrade
		i.Status.ResultStatus = cnab.StatusRunning
	})

	_, err := cp.GetInstallation(context.Background(), "", "wordpress")
	require.NoError(t, err, "ReadInstallation failed")

	displayInstall := NewDisplayInstallation(install)

	convertedInstallation, err := displayInstall.ConvertToInstallation()
	require.NoError(t, err, "failed to convert display installation to installation record")

	require.Equal(t, install.SchemaVersion, convertedInstallation.SchemaVersion, "invalid schema version")
	require.Equal(t, install.Name, convertedInstallation.Name, "invalid installation name")
	require.Equal(t, install.Namespace, convertedInstallation.Namespace, "invalid installation namespace")
	require.Equal(t, install.Uninstalled, convertedInstallation.Uninstalled, "invalid installation uninstalled status")
	require.Equal(t, install.Bundle.Digest, convertedInstallation.Bundle.Digest, "invalid installation bundle")

	require.Equal(t, len(install.Labels), len(convertedInstallation.Labels))
	for key := range displayInstall.Labels {
		require.Equal(t, install.Labels[key], convertedInstallation.Labels[key], "invalid installation labels")
	}

	require.Equal(t, install.Custom, convertedInstallation.Custom, "invalid installation custom")

	require.Equal(t, convertedInstallation.CredentialSets, install.CredentialSets, "invalid credential set")
	require.Equal(t, convertedInstallation.ParameterSets, install.ParameterSets, "invalid parameter set")

	require.Equal(t, install.Parameters.String(), convertedInstallation.Parameters.String(), "invalid parameters name")
	require.Equal(t, len(install.Parameters.Parameters), len(convertedInstallation.Parameters.Parameters))

	parametersMap := make(map[string]secrets.SourceMap, len(install.Parameters.Parameters))
	for _, param := range install.Parameters.Parameters {
		parametersMap[param.Name] = param
	}

	for _, param := range convertedInstallation.Parameters.Parameters {
		expected := parametersMap[param.Name]
		require.Equal(t, expected.ResolvedValue, param.ResolvedValue)
		expectedSource, err := expected.Source.MarshalJSON()
		require.NoError(t, err)
		source, err := param.Source.MarshalJSON()
		require.NoError(t, err)
		require.Equal(t, expectedSource, source)
	}

	require.Equal(t, install.Status.Created, convertedInstallation.Status.Created, "invalid created time")
	require.Equal(t, install.Status.Modified, convertedInstallation.Status.Modified, "invalid modified time")
	require.Equal(t, cnab.ActionUpgrade, convertedInstallation.Status.Action, "invalid last action")
	require.Equal(t, cnab.StatusRunning, convertedInstallation.Status.ResultStatus, "invalid last status")

}

func TestPorter_PrintInstallations(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name       string
		format     printer.Format
		outputFile string
	}{
		{name: "plain", format: printer.FormatPlaintext, outputFile: "testdata/list/expected-output.txt"},
		{name: "no reference, plain", format: printer.FormatPlaintext, outputFile: "testdata/list/no-reference-expected-output.txt"},
		{name: "json", format: printer.FormatJson, outputFile: "testdata/list/expected-output.json"},
		{name: "yaml", format: printer.FormatYaml, outputFile: "testdata/list/expected-output.yaml"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			p := NewTestPorter(t)
			defer p.Close()

			opts := ListOptions{
				Namespace: "dev",
				Name:      "mywordpress",
				PrintOptions: printer.PrintOptions{
					Format: tc.format,
				},
			}

			p.TestInstallations.CreateInstallation(storage.NewInstallation("dev", "mywordpress"), p.TestInstallations.SetMutableInstallationValues, func(i *storage.Installation) {
				i.Status.BundleVersion = "v1.2.3"
				i.Status.ResultStatus = cnab.StatusSucceeded
			})

			ctx := context.Background()

			err := p.PrintInstallations(ctx, opts)
			require.NoError(t, err, "PrintInstallation failed")
			p.CompareGoldenFile(tc.outputFile, p.TestConfig.TestContext.GetOutput())
		})
	}
}

func TestPorter_getDisplayInstallationState(t *testing.T) {
	p := NewTestPorter(t)
	defer p.Close()

	installation := p.TestInstallations.CreateInstallation(storage.NewInstallation("dev", "mywordpress"), p.TestInstallations.SetMutableInstallationValues)
	displayInstallationState := getDisplayInstallationState(installation)
	require.Equal(t, StateDefined, displayInstallationState)

	bun := cnab.ExtendedBundle{}
	run := p.TestInstallations.CreateRun(installation.NewRun(cnab.ActionInstall, bun), p.TestInstallations.SetMutableRunValues)
	result := p.TestInstallations.CreateResult(run.NewResult(cnab.StatusSucceeded), p.TestInstallations.SetMutableResultValues)
	installation.ApplyResult(run, result)
	installTime := now.Add(-time.Second * 5)
	installation.Status.Installed = &installTime
	displayInstallationState = getDisplayInstallationState(installation)
	require.Equal(t, StateInstalled, displayInstallationState)

	run = p.TestInstallations.CreateRun(installation.NewRun(cnab.ActionUninstall, bun), p.TestInstallations.SetMutableRunValues)
	result = p.TestInstallations.CreateResult(run.NewResult(cnab.StatusSucceeded), p.TestInstallations.SetMutableResultValues)
	installation.ApplyResult(run, result)
	installation.Status.Uninstalled = &now
	displayInstallationState = getDisplayInstallationState(installation)
	require.Equal(t, StateUninstalled, displayInstallationState)
}

func TestPorter_getDisplayInstallationStatus(t *testing.T) {
	p := NewTestPorter(t)
	defer p.Close()

	installation := p.TestInstallations.CreateInstallation(storage.NewInstallation("dev", "mywordpress"), p.TestInstallations.SetMutableInstallationValues)
	bun := cnab.ExtendedBundle{}
	run := p.TestInstallations.CreateRun(installation.NewRun(cnab.ActionInstall, bun), p.TestInstallations.SetMutableRunValues)
	result := p.TestInstallations.CreateResult(run.NewResult(cnab.StatusSucceeded), p.TestInstallations.SetMutableResultValues)
	installation.ApplyResult(run, result)
	displayInstallationStatus := getDisplayInstallationStatus(installation)
	require.Equal(t, cnab.StatusSucceeded, displayInstallationStatus)

	result = p.TestInstallations.CreateResult(run.NewResult(cnab.StatusFailed), p.TestInstallations.SetMutableResultValues)
	installation.ApplyResult(run, result)
	displayInstallationStatus = getDisplayInstallationStatus(installation)
	require.Equal(t, cnab.StatusFailed, displayInstallationStatus)

	run = p.TestInstallations.CreateRun(installation.NewRun(cnab.ActionInstall, bun), p.TestInstallations.SetMutableRunValues)
	result = p.TestInstallations.CreateResult(run.NewResult(cnab.StatusRunning), p.TestInstallations.SetMutableResultValues)
	installation.ApplyResult(run, result)
	displayInstallationStatus = getDisplayInstallationStatus(installation)
	require.Equal(t, StatusInstalling, displayInstallationStatus)

	run = p.TestInstallations.CreateRun(installation.NewRun(cnab.ActionUninstall, bun), p.TestInstallations.SetMutableRunValues)
	result = p.TestInstallations.CreateResult(run.NewResult(cnab.StatusRunning), p.TestInstallations.SetMutableResultValues)
	installation.ApplyResult(run, result)
	displayInstallationStatus = getDisplayInstallationStatus(installation)
	require.Equal(t, StatusUninstalling, displayInstallationStatus)

	run = p.TestInstallations.CreateRun(installation.NewRun(cnab.ActionUpgrade, bun), p.TestInstallations.SetMutableRunValues)
	result = p.TestInstallations.CreateResult(run.NewResult(cnab.StatusRunning), p.TestInstallations.SetMutableResultValues)
	installation.ApplyResult(run, result)
	displayInstallationStatus = getDisplayInstallationStatus(installation)
	require.Equal(t, StatusUpgrading, displayInstallationStatus)

	run = p.TestInstallations.CreateRun(installation.NewRun("customaction", bun), p.TestInstallations.SetMutableRunValues)
	result = p.TestInstallations.CreateResult(run.NewResult(cnab.StatusRunning), p.TestInstallations.SetMutableResultValues)
	installation.ApplyResult(run, result)
	installation.Status.Action = "customaction"
	displayInstallationStatus = getDisplayInstallationStatus(installation)
	require.Equal(t, "running customaction", displayInstallationStatus)
}

func Test_parseFieldSelector(t *testing.T) {
	tests := []struct {
		name           string
		fieldSelector  string
		want           map[string]string
		wantErr        bool
		expectedErrMsg string
	}{
		{
			name:          "valid field selector",
			fieldSelector: "bundle.version=0.2.0,status.action=install",
			want: map[string]string{
				"bundle.version": "0.2.0",
				"status.action":  "install",
			},
			wantErr: false,
		},
		{
			name:           "invalid field selector",
			fieldSelector:  "bundle.version=0.2.0,status.action",
			want:           nil,
			wantErr:        true,
			expectedErrMsg: "invalid field selector: bundle.version=0.2.0,status.action",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFieldSelector(tt.fieldSelector)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseFieldSelector() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseFieldSelector() = %v, want %v", got, tt.want)
			}
			if err != nil && err.Error() != tt.expectedErrMsg {
				t.Errorf("parseFieldSelector() error message = %v, expected error message %v", err.Error(), tt.expectedErrMsg)
			}
		})
	}
}

func Test_doesInstallationMatchFieldSelectors(t *testing.T) {
	type args struct {
		installation     DisplayInstallation
		fieldSelectorMap map[string]string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "installation matches field selectors",
			args: args{
				installation: DisplayInstallation{
					Name: "wordpress",
					Status: storage.InstallationStatus{
						Action:       "install",
						ResultStatus: "succeeded",
					},
					Bundle: storage.OCIReferenceParts{
						Version: "0.2.0",
					},
				},
				fieldSelectorMap: map[string]string{
					"name":                "wordpress",
					"status.action":       "install",
					"status.resultStatus": "succeeded",
					"bundle.version":      "0.2.0",
				},
			},
			want: true,
		},
		{
			name: "installation matches field selectors",
			args: args{
				installation: DisplayInstallation{
					Name: "wordpress",
					Status: storage.InstallationStatus{
						Action:       "install",
						ResultStatus: "succeeded",
					},
					Bundle: storage.OCIReferenceParts{
						Version: "0.2.1",
					},
				},
				fieldSelectorMap: map[string]string{
					"name":                "wordpress",
					"status.action":       "install",
					"status.resultStatus": "succeeded",
					"bundle.version":      "0.2.0",
				},
			},
			want: false,
		},
		{
			name: "installation matches field selectors",
			args: args{
				installation: DisplayInstallation{
					Name: "wordpress",
					Status: storage.InstallationStatus{
						Action:       "install",
						ResultStatus: "failed",
					},
					Bundle: storage.OCIReferenceParts{
						Version: "0.2.0",
					},
				},
				fieldSelectorMap: map[string]string{
					"name":                "wordpress",
					"status.action":       "install",
					"status.resultStatus": "succeeded",
					"bundle.version":      "0.2.0",
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := doesInstallationMatchFieldSelectors(tt.args.installation, tt.args.fieldSelectorMap); got != tt.want {
				t.Errorf("doesInstallationMatchFieldSelectors() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_installationHasFieldWithValue(t *testing.T) {
	type args struct {
		installation     DisplayInstallation
		fieldJsonTagPath string
		value            string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "installation has field with value",
			args: args{
				installation: DisplayInstallation{
					Name: "wordpress",
					Status: storage.InstallationStatus{
						Action:       "install",
						ResultStatus: "succeeded",
					},
					Bundle: storage.OCIReferenceParts{
						Version: "0.2.0",
					},
				},
				fieldJsonTagPath: "name",
				value:            "wordpress",
			},
			want: true,
		},
		{
			name: "installation has nested field with value",
			args: args{
				installation: DisplayInstallation{
					Name: "wordpress",
					Status: storage.InstallationStatus{
						Action:       "install",
						ResultStatus: "succeeded",
					},
					Bundle: storage.OCIReferenceParts{
						Version: "0.2.0",
					},
				},
				fieldJsonTagPath: "bundle.version",
				value:            "0.2.0",
			},
			want: true,
		},
		{
			name: "installation does not have field with value",
			args: args{
				installation: DisplayInstallation{
					Name: "wordpress",
					Status: storage.InstallationStatus{
						Action:       "install",
						ResultStatus: "succeeded",
					},
					Bundle: storage.OCIReferenceParts{
						Version: "0.2.0",
					},
				},
				fieldJsonTagPath: "bundle.version",
				value:            "0.2.1",
			},
			want: false,
		},
		{
			name: "installation does not have field with value",
			args: args{
				installation: DisplayInstallation{
					Name: "wordpress",
					Status: storage.InstallationStatus{
						Action:       "install",
						ResultStatus: "succeeded",
					},
					Bundle: storage.OCIReferenceParts{
						Version: "0.2.0",
					},
				},
				fieldJsonTagPath: "xyz",
				value:            "123",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := installationHasFieldWithValue(tt.args.installation, tt.args.fieldJsonTagPath, tt.args.value); got != tt.want {
				t.Errorf("installationHasFieldWithValue() = %v, want %v", got, tt.want)
			}
		})
	}
}
