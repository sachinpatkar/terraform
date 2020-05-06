package command

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/zclconf/go-cty/cty"

	"github.com/hashicorp/terraform/configs/configschema"
	"github.com/hashicorp/terraform/helper/copy"
	"github.com/hashicorp/terraform/providers"
	"github.com/hashicorp/terraform/terraform"
	"github.com/hashicorp/terraform/tfdiags"
)

func TestImport(t *testing.T) {
	defer testChdir(t, testFixturePath("import-provider-implicit"))()

	statePath := testTempFile(t)

	p := testProvider()
	ui := new(cli.MockUi)
	c := &ImportCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Ui:               ui,
		},
	}

	p.ImportResourceStateFn = nil
	p.ImportResourceStateResponse = providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_instance",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("yay"),
				}),
			},
		},
	}
	p.GetSchemaReturn = &terraform.ProviderSchema{
		ResourceTypes: map[string]*configschema.Block{
			"test_instance": {
				Attributes: map[string]*configschema.Attribute{
					"id": {Type: cty.String, Optional: true, Computed: true},
				},
			},
		},
	}

	args := []string{
		"-state", statePath,
		"test_instance.foo",
		"bar",
	}
	if code := c.Run(args); code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, ui.ErrorWriter.String())
	}

	if !p.ImportResourceStateCalled {
		t.Fatal("ImportResourceState should be called")
	}

	testStateOutput(t, statePath, testImportStr)
}

func TestImport_providerConfig(t *testing.T) {
	defer testChdir(t, testFixturePath("import-provider"))()

	statePath := testTempFile(t)

	p := testProvider()
	ui := new(cli.MockUi)
	c := &ImportCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Ui:               ui,
		},
	}

	p.ImportResourceStateFn = nil
	p.ImportResourceStateResponse = providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_instance",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("yay"),
				}),
			},
		},
	}
	p.GetSchemaReturn = &terraform.ProviderSchema{
		Provider: &configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"foo": {Type: cty.String, Optional: true},
			},
		},
		ResourceTypes: map[string]*configschema.Block{
			"test_instance": {
				Attributes: map[string]*configschema.Attribute{
					"id": {Type: cty.String, Optional: true, Computed: true},
				},
			},
		},
	}

	configured := false
	p.ConfigureNewFn = func(req providers.ConfigureRequest) providers.ConfigureResponse {
		configured = true

		cfg := req.Config
		if !cfg.Type().HasAttribute("foo") {
			return providers.ConfigureResponse{
				Diagnostics: tfdiags.Diagnostics{}.Append(fmt.Errorf("configuration has no foo argument")),
			}
		}
		if got, want := cfg.GetAttr("foo"), cty.StringVal("bar"); !want.RawEquals(got) {
			return providers.ConfigureResponse{
				Diagnostics: tfdiags.Diagnostics{}.Append(fmt.Errorf("foo argument is %#v, but want %#v", got, want)),
			}
		}

		return providers.ConfigureResponse{}
	}

	args := []string{
		"-state", statePath,
		"test_instance.foo",
		"bar",
	}
	if code := c.Run(args); code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, ui.ErrorWriter.String())
	}

	// Verify that we were called
	if !configured {
		t.Fatal("Configure should be called")
	}

	if !p.ImportResourceStateCalled {
		t.Fatal("ImportResourceState should be called")
	}

	testStateOutput(t, statePath, testImportStr)
}

// "remote" state provided by the "local" backend
func TestImport_remoteState(t *testing.T) {
	td := tempDir(t)
	copy.CopyDir(testFixturePath("import-provider-remote-state"), td)
	defer os.RemoveAll(td)
	defer testChdir(t, td)()

	statePath := "imported.tfstate"

	providerSource, close := newMockProviderSource(t, map[string][]string{
		"test": []string{"1.2.3"},
	})
	defer close()

	// init our backend
	ui := cli.NewMockUi()
	m := Meta{
		testingOverrides: metaOverridesForProvider(testProvider()),
		Ui:               ui,
		ProviderSource:   providerSource,
	}

	ic := &InitCommand{
		Meta: m,
	}

	// (Using log here rather than t.Log so that these messages interleave with other trace logs)
	log.Print("[TRACE] TestImport_remoteState running: terraform init")
	if code := ic.Run([]string{}); code != 0 {
		t.Fatalf("init failed\n%s", ui.ErrorWriter)
	}

	p := testProvider()
	ui = new(cli.MockUi)
	c := &ImportCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Ui:               ui,
		},
	}

	p.ImportResourceStateFn = nil
	p.ImportResourceStateResponse = providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_instance",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("yay"),
				}),
			},
		},
	}
	p.GetSchemaReturn = &terraform.ProviderSchema{
		Provider: &configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"foo": {Type: cty.String, Optional: true},
			},
		},
		ResourceTypes: map[string]*configschema.Block{
			"test_instance": {
				Attributes: map[string]*configschema.Attribute{
					"id": {Type: cty.String, Optional: true, Computed: true},
				},
			},
		},
	}

	configured := false
	p.ConfigureNewFn = func(req providers.ConfigureRequest) providers.ConfigureResponse {
		var diags tfdiags.Diagnostics
		configured = true
		if got, want := req.Config.GetAttr("foo"), cty.StringVal("bar"); !want.RawEquals(got) {
			diags = diags.Append(fmt.Errorf("wrong \"foo\" value %#v; want %#v", got, want))
		}
		return providers.ConfigureResponse{
			Diagnostics: diags,
		}
	}

	args := []string{
		"test_instance.foo",
		"bar",
	}
	log.Printf("[TRACE] TestImport_remoteState running: terraform import %s %s", args[0], args[1])
	if code := c.Run(args); code != 0 {
		fmt.Println(ui.OutputWriter)
		t.Fatalf("bad: %d\n\n%s", code, ui.ErrorWriter.String())
	}

	// verify that the local state was unlocked after import
	if _, err := os.Stat(filepath.Join(td, fmt.Sprintf(".%s.lock.info", statePath))); !os.IsNotExist(err) {
		t.Fatal("state left locked after import")
	}

	// Verify that we were called
	if !configured {
		t.Fatal("Configure should be called")
	}

	if !p.ImportResourceStateCalled {
		t.Fatal("ImportResourceState should be called")
	}

	testStateOutput(t, statePath, testImportStr)
}

// early failure on import should not leave stale lock
func TestImport_initializationErrorShouldUnlock(t *testing.T) {
	td := tempDir(t)
	copy.CopyDir(testFixturePath("import-provider-remote-state"), td)
	defer os.RemoveAll(td)
	defer testChdir(t, td)()

	statePath := "imported.tfstate"

	providerSource, close := newMockProviderSource(t, map[string][]string{
		"test": []string{"1.2.3"},
	})
	defer close()

	// init our backend
	ui := cli.NewMockUi()
	m := Meta{
		testingOverrides: metaOverridesForProvider(testProvider()),
		Ui:               ui,
		ProviderSource:   providerSource,
	}

	ic := &InitCommand{
		Meta: m,
	}

	// (Using log here rather than t.Log so that these messages interleave with other trace logs)
	log.Print("[TRACE] TestImport_initializationErrorShouldUnlock running: terraform init")
	if code := ic.Run([]string{}); code != 0 {
		t.Fatalf("init failed\n%s", ui.ErrorWriter)
	}

	// overwrite the config with one including a resource from an invalid provider
	copy.CopyFile(filepath.Join(testFixturePath("import-provider-invalid"), "main.tf"), filepath.Join(td, "main.tf"))

	p := testProvider()
	ui = new(cli.MockUi)
	c := &ImportCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Ui:               ui,
		},
	}

	args := []string{
		"unknown_instance.baz",
		"bar",
	}
	log.Printf("[TRACE] TestImport_initializationErrorShouldUnlock running: terraform import %s %s", args[0], args[1])

	// this should fail
	if code := c.Run(args); code != 1 {
		fmt.Println(ui.OutputWriter)
		t.Fatalf("bad: %d\n\n%s", code, ui.ErrorWriter.String())
	}

	// specifically, it should fail due to a missing provider
	msg := ui.ErrorWriter.String()
	if want := `unknown provider "registry.terraform.io/hashicorp/unknown"`; !strings.Contains(msg, want) {
		t.Errorf("incorrect message\nwant substring: %s\ngot:\n%s", want, msg)
	}

	// verify that the local state was unlocked after initialization error
	if _, err := os.Stat(filepath.Join(td, fmt.Sprintf(".%s.lock.info", statePath))); !os.IsNotExist(err) {
		t.Fatal("state left locked after import")
	}
}

func TestImport_providerConfigWithVar(t *testing.T) {
	defer testChdir(t, testFixturePath("import-provider-var"))()

	statePath := testTempFile(t)

	p := testProvider()
	ui := new(cli.MockUi)
	c := &ImportCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Ui:               ui,
		},
	}

	p.ImportResourceStateFn = nil
	p.ImportResourceStateResponse = providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_instance",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("yay"),
				}),
			},
		},
	}
	p.GetSchemaReturn = &terraform.ProviderSchema{
		Provider: &configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"foo": {Type: cty.String, Optional: true},
			},
		},
		ResourceTypes: map[string]*configschema.Block{
			"test_instance": {
				Attributes: map[string]*configschema.Attribute{
					"id": {Type: cty.String, Optional: true, Computed: true},
				},
			},
		},
	}

	configured := false
	p.ConfigureNewFn = func(req providers.ConfigureRequest) providers.ConfigureResponse {
		var diags tfdiags.Diagnostics
		configured = true
		if got, want := req.Config.GetAttr("foo"), cty.StringVal("bar"); !want.RawEquals(got) {
			diags = diags.Append(fmt.Errorf("wrong \"foo\" value %#v; want %#v", got, want))
		}
		return providers.ConfigureResponse{
			Diagnostics: diags,
		}
	}

	args := []string{
		"-state", statePath,
		"-var", "foo=bar",
		"test_instance.foo",
		"bar",
	}
	if code := c.Run(args); code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, ui.ErrorWriter.String())
	}

	// Verify that we were called
	if !configured {
		t.Fatal("Configure should be called")
	}

	if !p.ImportResourceStateCalled {
		t.Fatal("ImportResourceState should be called")
	}

	testStateOutput(t, statePath, testImportStr)
}

func TestImport_providerConfigWithDataSource(t *testing.T) {
	defer testChdir(t, testFixturePath("import-provider-datasource"))()

	statePath := testTempFile(t)

	p := testProvider()
	ui := new(cli.MockUi)
	c := &ImportCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Ui:               ui,
		},
	}

	p.ImportResourceStateFn = nil
	p.ImportResourceStateResponse = providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_instance",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("yay"),
				}),
			},
		},
	}
	p.GetSchemaReturn = &terraform.ProviderSchema{
		Provider: &configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"foo": {Type: cty.String, Optional: true},
			},
		},
		ResourceTypes: map[string]*configschema.Block{
			"test_instance": {
				Attributes: map[string]*configschema.Attribute{
					"id": {Type: cty.String, Optional: true, Computed: true},
				},
			},
		},
		DataSources: map[string]*configschema.Block{
			"test_data": {
				Attributes: map[string]*configschema.Attribute{
					"id": {Type: cty.String, Optional: true, Computed: true},
				},
			},
		},
	}

	args := []string{
		"-state", statePath,
		"test_instance.foo",
		"bar",
	}
	if code := c.Run(args); code != 1 {
		t.Fatalf("bad, wanted error: %d\n\n%s", code, ui.ErrorWriter.String())
	}
}

func TestImport_providerConfigWithVarDefault(t *testing.T) {
	defer testChdir(t, testFixturePath("import-provider-var-default"))()

	statePath := testTempFile(t)

	p := testProvider()
	ui := new(cli.MockUi)
	c := &ImportCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Ui:               ui,
		},
	}

	p.ImportResourceStateFn = nil
	p.ImportResourceStateResponse = providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_instance",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("yay"),
				}),
			},
		},
	}
	p.GetSchemaReturn = &terraform.ProviderSchema{
		Provider: &configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"foo": {Type: cty.String, Optional: true},
			},
		},
		ResourceTypes: map[string]*configschema.Block{
			"test_instance": {
				Attributes: map[string]*configschema.Attribute{
					"id": {Type: cty.String, Optional: true, Computed: true},
				},
			},
		},
	}

	configured := false
	p.ConfigureNewFn = func(req providers.ConfigureRequest) providers.ConfigureResponse {
		var diags tfdiags.Diagnostics
		configured = true
		if got, want := req.Config.GetAttr("foo"), cty.StringVal("bar"); !want.RawEquals(got) {
			diags = diags.Append(fmt.Errorf("wrong \"foo\" value %#v; want %#v", got, want))
		}
		return providers.ConfigureResponse{
			Diagnostics: diags,
		}
	}

	args := []string{
		"-state", statePath,
		"test_instance.foo",
		"bar",
	}
	if code := c.Run(args); code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, ui.ErrorWriter.String())
	}

	// Verify that we were called
	if !configured {
		t.Fatal("Configure should be called")
	}

	if !p.ImportResourceStateCalled {
		t.Fatal("ImportResourceState should be called")
	}

	testStateOutput(t, statePath, testImportStr)
}

func TestImport_providerConfigWithVarFile(t *testing.T) {
	defer testChdir(t, testFixturePath("import-provider-var-file"))()

	statePath := testTempFile(t)

	p := testProvider()
	ui := new(cli.MockUi)
	c := &ImportCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Ui:               ui,
		},
	}

	p.ImportResourceStateFn = nil
	p.ImportResourceStateResponse = providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_instance",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("yay"),
				}),
			},
		},
	}
	p.GetSchemaReturn = &terraform.ProviderSchema{
		Provider: &configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"foo": {Type: cty.String, Optional: true},
			},
		},
		ResourceTypes: map[string]*configschema.Block{
			"test_instance": {
				Attributes: map[string]*configschema.Attribute{
					"id": {Type: cty.String, Optional: true, Computed: true},
				},
			},
		},
	}

	configured := false
	p.ConfigureNewFn = func(req providers.ConfigureRequest) providers.ConfigureResponse {
		var diags tfdiags.Diagnostics
		configured = true
		if got, want := req.Config.GetAttr("foo"), cty.StringVal("bar"); !want.RawEquals(got) {
			diags = diags.Append(fmt.Errorf("wrong \"foo\" value %#v; want %#v", got, want))
		}
		return providers.ConfigureResponse{
			Diagnostics: diags,
		}
	}

	args := []string{
		"-state", statePath,
		"-var-file", "blah.tfvars",
		"test_instance.foo",
		"bar",
	}
	if code := c.Run(args); code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, ui.ErrorWriter.String())
	}

	// Verify that we were called
	if !configured {
		t.Fatal("Configure should be called")
	}

	if !p.ImportResourceStateCalled {
		t.Fatal("ImportResourceState should be called")
	}

	testStateOutput(t, statePath, testImportStr)
}

func TestImport_disallowMissingResourceConfig(t *testing.T) {
	defer testChdir(t, testFixturePath("import-missing-resource-config"))()

	statePath := testTempFile(t)

	p := testProvider()
	ui := new(cli.MockUi)
	c := &ImportCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Ui:               ui,
		},
	}

	p.ImportResourceStateFn = nil
	p.ImportResourceStateResponse = providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_instance",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("yay"),
				}),
			},
		},
	}
	p.GetSchemaReturn = &terraform.ProviderSchema{
		ResourceTypes: map[string]*configschema.Block{
			"test_instance": {
				Attributes: map[string]*configschema.Attribute{
					"id": {Type: cty.String, Optional: true, Computed: true},
				},
			},
		},
	}

	args := []string{
		"-state", statePath,
		"-allow-missing-config",
		"test_instance.foo",
		"bar",
	}

	if code := c.Run(args); code != 1 {
		t.Fatalf("import succeeded; expected failure")
	}

	msg := ui.ErrorWriter.String()

	if want := `Error: Resource test_instance.foo not found in the configuration.`; !strings.Contains(msg, want) {
		t.Errorf("incorrect message\nwant substring: %s\ngot:\n%s", want, msg)
	}
}

func TestImport_emptyConfig(t *testing.T) {
	defer testChdir(t, testFixturePath("empty"))()

	statePath := testTempFile(t)

	p := testProvider()
	ui := new(cli.MockUi)
	c := &ImportCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Ui:               ui,
		},
	}

	args := []string{
		"-state", statePath,
		"test_instance.foo",
		"bar",
	}
	code := c.Run(args)
	if code != 1 {
		t.Fatalf("import succeeded; expected failure")
	}

	msg := ui.ErrorWriter.String()
	if want := `No Terraform configuration files`; !strings.Contains(msg, want) {
		t.Errorf("incorrect message\nwant substring: %s\ngot:\n%s", want, msg)
	}
}

func TestImport_missingResourceConfig(t *testing.T) {
	defer testChdir(t, testFixturePath("import-missing-resource-config"))()

	statePath := testTempFile(t)

	p := testProvider()
	ui := new(cli.MockUi)
	c := &ImportCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Ui:               ui,
		},
	}

	args := []string{
		"-state", statePath,
		"test_instance.foo",
		"bar",
	}
	code := c.Run(args)
	if code != 1 {
		t.Fatalf("import succeeded; expected failure")
	}

	msg := ui.ErrorWriter.String()
	if want := `resource address "test_instance.foo" does not exist`; !strings.Contains(msg, want) {
		t.Errorf("incorrect message\nwant substring: %s\ngot:\n%s", want, msg)
	}
}

func TestImport_missingModuleConfig(t *testing.T) {
	defer testChdir(t, testFixturePath("import-missing-resource-config"))()

	statePath := testTempFile(t)

	p := testProvider()
	ui := new(cli.MockUi)
	c := &ImportCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Ui:               ui,
		},
	}

	args := []string{
		"-state", statePath,
		"module.baz.test_instance.foo",
		"bar",
	}
	code := c.Run(args)
	if code != 1 {
		t.Fatalf("import succeeded; expected failure")
	}

	msg := ui.ErrorWriter.String()
	if want := `module.baz is not defined in the configuration`; !strings.Contains(msg, want) {
		t.Errorf("incorrect message\nwant substring: %s\ngot:\n%s", want, msg)
	}
}

func TestImport_dataResource(t *testing.T) {
	defer testChdir(t, testFixturePath("import-missing-resource-config"))()

	statePath := testTempFile(t)

	p := testProvider()
	ui := new(cli.MockUi)
	c := &ImportCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Ui:               ui,
		},
	}

	args := []string{
		"-state", statePath,
		"data.test_data_source.foo",
		"bar",
	}
	code := c.Run(args)
	if code != 1 {
		t.Fatalf("import succeeded; expected failure")
	}

	msg := ui.ErrorWriter.String()
	if want := `A managed resource address is required`; !strings.Contains(msg, want) {
		t.Errorf("incorrect message\nwant substring: %s\ngot:\n%s", want, msg)
	}
}

func TestImport_invalidResourceAddr(t *testing.T) {
	defer testChdir(t, testFixturePath("import-missing-resource-config"))()

	statePath := testTempFile(t)

	p := testProvider()
	ui := new(cli.MockUi)
	c := &ImportCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Ui:               ui,
		},
	}

	args := []string{
		"-state", statePath,
		"bananas",
		"bar",
	}
	code := c.Run(args)
	if code != 1 {
		t.Fatalf("import succeeded; expected failure")
	}

	msg := ui.ErrorWriter.String()
	if want := `Error: Invalid address`; !strings.Contains(msg, want) {
		t.Errorf("incorrect message\nwant substring: %s\ngot:\n%s", want, msg)
	}
}

func TestImport_targetIsModule(t *testing.T) {
	defer testChdir(t, testFixturePath("import-missing-resource-config"))()

	statePath := testTempFile(t)

	p := testProvider()
	ui := new(cli.MockUi)
	c := &ImportCommand{
		Meta: Meta{
			testingOverrides: metaOverridesForProvider(p),
			Ui:               ui,
		},
	}

	args := []string{
		"-state", statePath,
		"module.foo",
		"bar",
	}
	code := c.Run(args)
	if code != 1 {
		t.Fatalf("import succeeded; expected failure")
	}

	msg := ui.ErrorWriter.String()
	if want := `Error: Invalid address`; !strings.Contains(msg, want) {
		t.Errorf("incorrect message\nwant substring: %s\ngot:\n%s", want, msg)
	}
}

const testImportStr = `
test_instance.foo:
  ID = yay
  provider = provider["registry.terraform.io/hashicorp/test"]
`

const testImportCustomProviderStr = `
test_instance.foo:
  ID = yay
  provider = provider["registry.terraform.io/hashicorp/test"].alias
`

const testImportProviderMismatchStr = `
test_instance.foo:
  ID = yay
  provider = provider["registry.terraform.io/hashicorp/test-beta"]
`
