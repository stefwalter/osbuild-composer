// +build integration

package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/osbuild/osbuild-composer/internal/blueprint"

	"github.com/stretchr/testify/require"
	"testing"
)

func TestComposeCommands(t *testing.T) {
	// common setup
	tmpdir := NewTemporaryWorkDir(t, "osbuild-tests-")
	defer tmpdir.Close(t)

	bp := blueprint.Blueprint{
		Name:        "empty",
		Description: "Test empty blueprint in toml format",
	}
	pushBlueprint(t, &bp)
	defer deleteBlueprint(t, &bp)

	runComposerCLIPlainText(t, "blueprints", "save", "empty")
	_, err := os.Stat("empty.toml")
	require.NoError(t, err, "Error accessing 'empty.toml: %v'", err)

	runComposerCLIPlainText(t, "compose", "types")
	runComposerCLIPlainText(t, "compose", "status")
	runComposerCLIPlainText(t, "compose", "list")
	runComposerCLIPlainText(t, "compose", "list", "waiting")
	runComposerCLIPlainText(t, "compose", "list", "running")
	runComposerCLIPlainText(t, "compose", "list", "finished")
	runComposerCLIPlainText(t, "compose", "list", "failed")

	// Full integration tests
	uuid := buildCompose(t, "empty", "ami")
	defer deleteCompose(t, uuid)

	runComposerCLIPlainText(t, "compose", "info", uuid.String())

	// https://github.com/osbuild/osbuild-composer/issues/643
	// runComposerCLIPlainText(t, "compose", "metadata", uuid.String())
	// _, err := os.Stat(uuid.String() + "-metadata.tar")
	// require.NoError(t, err, "'%s-metadata.tar' not found", uuid.String())
	// defer os.Remove(uuid.String() + "-metadata.tar")

	// https://github.com/osbuild/osbuild-composer/issues/644
	// runComposerCLIPlainText(t, "compose", "results", uuid.String())
	// _, err = os.Stat(uuid.String() + ".tar")
	// require.NoError(t, err, "'%s.tar' not found", uuid.String())
	// defer os.Remove(uuid.String() + ".tar")

	// Just assert that result wasn't empty
	result := runComposerCLIPlainText(t, "compose", "log", uuid.String())
	require.NotNil(t, result)
	result = runComposerCLIPlainText(t, "compose", "log", uuid.String(), "1024")
	require.NotNil(t, result)

	runComposerCLIPlainText(t, "compose", "logs", uuid.String())
	_, err = os.Stat(uuid.String() + "-logs.tar")
	require.NoError(t, err, "'%s-logs.tar' not found", uuid.String())
	defer os.Remove(uuid.String() + "-logs.tar")

	runComposerCLIPlainText(t, "compose", "image", uuid.String())
	_, err = os.Stat(uuid.String() + "-image.vhdx")
	require.NoError(t, err, "'%s-image.vhdx' not found", uuid.String())
	defer os.Remove(uuid.String() + "-image.vhdx")

	// https://github.com/osbuild/osbuild-composer/pull/180
	// uuid = startCompose(t, "empty", "tar")
	// time.Sleep(time.Second)
	// runComposerCLIPlainText(t, "compose", "cancel", uuid.String())
}

func TestEverything(t *testing.T) {
	runComposerCLI(t, false, "blueprints", "list")
	// runCommand(false, "blueprints", "show", BLUEPRINT,....)
	// runCommand(false, "blueprints", "changes", BLUEPRINT,....)
	// runCommand(false, "blueprints", "diff", BLUEPRINT, FROM/NEWEST, TO/NEWEST/WORKSPACE)
	// runCommand(false, "blueprints", "save", BLUEPRINT,...)
	// runCommand(false, "blueprints", "delete", BLUEPRINT)
	// runCommand(false, "blueprints", "depsolve", BLUEPRINT,...)
	// runCommand(false, "blueprints", "push", BLUEPRINT.TOML)
	// runCommand(false, "blueprints", "freeze", BLUEPRINT,...)
	// runCommand(false, "blueprints", "freeze", "show", BLUEPRINT,...)
	// runCommand(false, "blueprints", "freeze", "save", BLUEPRINT,...)
	// runCommand(false, "blueprints", "tag", BLUEPRINT)
	// runCommand(false, "blueprints", "undo", BLUEPRINT, COMMIT)
	// runCommand(false, "blueprints", "workspace", BLUEPRINT)
	runComposerCLI(t, false, "modules", "list")
	runComposerCLI(t, false, "projects", "list")
	runComposerCLI(t, false, "projects", "info", "filesystem")
	runComposerCLI(t, false, "projects", "info", "filesystem", "kernel")
	runComposerCLI(t, false, "status", "show")
}

func TestSourcesCommands(t *testing.T) {
	sources_toml, err := ioutil.TempFile("", "SOURCES-*.TOML")
	require.NoErrorf(t, err, "Could not create temporary file: %v", err)
	defer os.Remove(sources_toml.Name())

	_, err = sources_toml.Write([]byte(`name = "osbuild-test-addon-source"
url = "file://REPO-PATH"
type = "yum-baseurl"
proxy = "https://proxy-url/"
check_ssl = true
check_gpg = true
gpgkey_urls = ["https://url/path/to/gpg-key"]
`))
	require.NoError(t, err)

	runComposerCLI(t, false, "sources", "list")
	runComposerCLI(t, false, "sources", "add", sources_toml.Name())
	runComposerCLI(t, false, "sources", "info", "osbuild-test-addon-source")
	runComposerCLI(t, false, "sources", "change", sources_toml.Name())
	runComposerCLI(t, false, "sources", "delete", "osbuild-test-addon-source")
}

func buildCompose(t *testing.T, bpName string, outputType string) uuid.UUID {
	uuid := startCompose(t, bpName, outputType)
	status := waitForCompose(t, uuid)
	logs := getLogs(t, uuid)
	assert.NotEmpty(t, logs, "logs are empty after the build is finished/failed")

	if !assert.Equalf(t, "FINISHED", status, "Unexpected compose result: %s", status) {
		log.Print("logs from the build: ", logs)
		t.FailNow()
	}

	return uuid
}

func startCompose(t *testing.T, name, outputType string) uuid.UUID {
	var reply struct {
		BuildID uuid.UUID `json:"build_id"`
		Status  bool      `json:"status"`
	}
	rawReply := runComposerCLI(t, false, "compose", "start", name, outputType)
	err := json.Unmarshal(rawReply, &reply)
	require.Nilf(t, err, "Unexpected reply: %v", err)
	require.Truef(t, reply.Status, "Unexpected status %v", reply.Status)

	return reply.BuildID
}

func deleteCompose(t *testing.T, id uuid.UUID) {
	type deleteUUID struct {
		ID     uuid.UUID `json:"uuid"`
		Status bool      `json:"status"`
	}
	var reply struct {
		IDs    []deleteUUID  `json:"uuids"`
		Errors []interface{} `json:"errors"`
	}
	rawReply := runComposerCLI(t, false, "compose", "delete", id.String())
	err := json.Unmarshal(rawReply, &reply)
	require.Nilf(t, err, "Unexpected reply: %v", err)
	require.Zerof(t, len(reply.Errors), "Unexpected errors")
	require.Equalf(t, 1, len(reply.IDs), "Unexpected number of UUIDs returned: %d", len(reply.IDs))
	require.Truef(t, reply.IDs[0].Status, "Unexpected status %v", reply.IDs[0].Status)
}

func waitForCompose(t *testing.T, uuid uuid.UUID) string {
	for {
		status := getComposeStatus(t, true, uuid)
		if status == "FINISHED" || status == "FAILED" {
			return status
		}
		time.Sleep(time.Second)
	}
}

func getComposeStatus(t *testing.T, quiet bool, uuid uuid.UUID) string {
	var reply struct {
		QueueStatus string `json:"queue_status"`
	}
	rawReply := runComposerCLI(t, quiet, "compose", "info", uuid.String())
	err := json.Unmarshal(rawReply, &reply)
	require.Nilf(t, err, "Unexpected reply: %v", err)

	return reply.QueueStatus
}

func getLogs(t *testing.T, uuid uuid.UUID) string {
	cmd := exec.Command("composer-cli", "compose", "log", uuid.String())
	cmd.Stderr = os.Stderr
	stdoutReader, err := cmd.StdoutPipe()
	require.NoError(t, err)

	err = cmd.Start()
	require.NoError(t, err)

	var buffer bytes.Buffer
	_, err = buffer.ReadFrom(stdoutReader)
	require.NoError(t, err)

	err = cmd.Wait()
	require.NoError(t, err)

	return buffer.String()
}

func pushBlueprint(t *testing.T, bp *blueprint.Blueprint) {
	tmpfile, err := ioutil.TempFile("", "osbuild-test-")
	require.Nilf(t, err, "Could not create temporary file: %v", err)
	defer os.Remove(tmpfile.Name())

	err = toml.NewEncoder(tmpfile).Encode(bp)
	require.Nilf(t, err, "Could not marshal blueprint TOML: %v", err)
	err = tmpfile.Close()
	require.Nilf(t, err, "Could not close toml file: %v", err)

	var reply struct {
		Status bool `json:"status"`
	}
	rawReply := runComposerCLI(t, false, "blueprints", "push", tmpfile.Name())
	err = json.Unmarshal(rawReply, &reply)
	require.Nilf(t, err, "Unexpected reply: %v", err)
	require.Truef(t, reply.Status, "Unexpected status %v", reply.Status)
}

func deleteBlueprint(t *testing.T, bp *blueprint.Blueprint) {
	var reply struct {
		Status bool `json:"status"`
	}
	rawReply := runComposerCLI(t, false, "blueprints", "delete", bp.Name)
	err := json.Unmarshal(rawReply, &reply)
	require.Nilf(t, err, "Unexpected reply: %v", err)
	require.Truef(t, reply.Status, "Unexpected status %v", reply.Status)
}

func runComposerCLIPlainText(t *testing.T, command ...string) []byte {
	cmd := exec.Command("composer-cli", command...)
	stdout, err := cmd.StdoutPipe()
	require.Nilf(t, err, "Could not create command: %v", err)

	err = cmd.Start()
	require.Nilf(t, err, "Could not start command: %v", err)

	contents, err := ioutil.ReadAll(stdout)
	require.NoError(t, err, "Could not read stdout from command")

	err = cmd.Wait()
	require.NoErrorf(t, err, "Command failed: %v", err)

	return contents
}

func runComposerCLI(t *testing.T, quiet bool, command ...string) json.RawMessage {
	command = append([]string{"--json"}, command...)
	contents := runComposerCLIPlainText(t, command...)

	var result json.RawMessage

	if len(contents) != 0 {
		err := json.Unmarshal(contents, &result)
		if err != nil {
			// We did not get JSON, try interpreting it as TOML
			var data interface{}
			err = toml.Unmarshal(contents, &data)
			require.Nilf(t, err, "Could not parse output: %v", err)
			buffer := bytes.Buffer{}
			err = json.NewEncoder(&buffer).Encode(data)
			require.Nilf(t, err, "Could not remarshal TOML to JSON: %v", err)
			err = json.NewDecoder(&buffer).Decode(&result)
			require.Nilf(t, err, "Could not decode the remarshalled JSON: %v", err)
		}
	}

	buffer := bytes.Buffer{}
	encoder := json.NewEncoder(&buffer)
	encoder.SetIndent("", "  ")
	err := encoder.Encode(result)
	require.Nilf(t, err, "Could not remarshal the recevied JSON: %v", err)

	return result
}

type TemporaryWorkDir struct {
	OldWorkDir string
	Path       string
}

// Creates a new temporary directory based on pattern and changes the current
// working directory to it.
//
// Example:
//   d := NewTemporaryWorkDir(t, "foo-*")
//   defer d.Close(t)
func NewTemporaryWorkDir(t *testing.T, pattern string) TemporaryWorkDir {
	var d TemporaryWorkDir
	var err error

	d.OldWorkDir, err = os.Getwd()
	require.Nilf(t, err, "os.GetWd: %v", err)

	d.Path, err = ioutil.TempDir("", pattern)
	require.Nilf(t, err, "ioutil.TempDir: %v", err)

	err = os.Chdir(d.Path)
	require.Nilf(t, err, "os.ChDir: %v", err)

	return d
}

// Change back to the previous working directory and removes the temporary one.
func (d *TemporaryWorkDir) Close(t *testing.T) {
	var err error

	err = os.Chdir(d.OldWorkDir)
	require.Nilf(t, err, "os.ChDir: %v", err)

	err = os.RemoveAll(d.Path)
	require.Nilf(t, err, "os.RemoveAll: %v", err)
}
