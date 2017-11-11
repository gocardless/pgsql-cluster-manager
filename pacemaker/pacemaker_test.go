package pacemaker

import (
	"errors"
	"io/ioutil"
	"testing"

	"golang.org/x/net/context"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type fakeExecutor struct{ mock.Mock }

func (e fakeExecutor) CombinedOutput(ctx context.Context, name string, arg ...string) ([]byte, error) {
	args := e.Called(ctx, name, arg)
	return args.Get(0).([]byte), args.Error(1)
}

func TestGet(t *testing.T) {
	testCases := []struct {
		name    string
		fixture string
		value   string
		err     error
	}{
		{
			"with sync / async / master",
			"./fixtures/cib_sync_async_master.xml",
			"pg03",
			nil,
		},
		{
			"with master / sync / died",
			"./fixtures/cib_master_sync_died.xml",
			"pg01",
			nil,
		},
		{
			"when we don't have quorum",
			"./fixtures/cib_master_died_died.xml",
			"",
			errors.New("no quorum"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fixture, fixtureErr := ioutil.ReadFile(tc.fixture)
			require.Nil(t, fixtureErr)

			executor := new(fakeExecutor)
			executor.On("CombinedOutput", context.Background(), "cibadmin", []string{"--query", "--local"}).Return(fixture, nil)

			nodes, err := Pacemaker{executor}.Get(context.Background(), MasterXPath)

			executor.AssertExpectations(t)

			if tc.err != nil {
				if assert.Error(t, err, "expected Get() to return error") {
					assert.Equal(t, tc.err.Error(), err.Error())
				}
			}

			if tc.value != "" {
				assert.Equal(t, 1, len(nodes), "expected only one node result")
				assert.Equal(t, tc.value, nodes[0].SelectAttrValue("uname", ""))
			}
		})
	}
}

func TestResolveAddress(t *testing.T) {
	testCases := []struct {
		name            string
		execOutput      []byte
		execErr         error
		expectedAddress string
		expectedErr     error
	}{
		{
			"when corosync-cfgtool returns no error",
			[]byte("172.17.0.4\n"),
			nil,
			"172.17.0.4",
			nil,
		},
		{
			"when corosync-cfgtool returns an error",
			[]byte(""),
			errors.New("exec: \"corosync-cfgtool\": executable file not found in $PATH"),
			"",
			errors.New("failed to run corosync-cfgtool: exec: \"corosync-cfgtool\": executable file not found in $PATH"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bgCtx := context.Background()

			executor := new(fakeExecutor)
			executor.
				On("CombinedOutput", bgCtx, "corosync-cfgtool", []string{"-a", "1"}).
				Return(tc.execOutput, tc.execErr)

			address, err := Pacemaker{executor}.ResolveAddress(bgCtx, "1")

			if err != nil {
				assert.EqualValues(t, tc.expectedErr.Error(), err.Error())
			}
			assert.EqualValues(t, tc.expectedAddress, address)
			executor.AssertExpectations(t)
		})
	}
}

func TestMigrate(t *testing.T) {
	testCases := []struct {
		name    string
		execErr error // returned from executor
		err     error // returned from Migrate
	}{
		{
			"when migrate returns no error",
			nil,
			nil,
		},
		{
			"when migrate returns an error",
			errors.New("exit 255"),
			errors.New("failed to execute crm migration: exit 255"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			executor := new(fakeExecutor)
			executor.On("CombinedOutput", context.Background(), "crm", []string{
				"resource", "migrate", "msPostgresql", "pg03",
			}).Return([]byte(""), tc.execErr)

			err := Pacemaker{executor}.Migrate(context.Background(), "pg03")

			if err != nil {
				assert.EqualValues(t, tc.err.Error(), err.Error())
			}
			executor.AssertExpectations(t)
		})
	}
}

func TestUnmigrate(t *testing.T) {
	testCases := []struct {
		name    string
		execErr error // returned from executor
		err     error // returned from Unmigrate
	}{
		{
			"when unmigrate returns no error",
			nil,
			nil,
		},
		{
			"when unmigrate returns an error",
			errors.New("exit 255"),
			errors.New("failed to execute crm resource unmigrate: exit 255"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			executor := new(fakeExecutor)
			executor.On("CombinedOutput", context.Background(), "crm", []string{
				"resource", "unmigrate", "msPostgresql",
			}).Return([]byte(""), tc.execErr)

			err := Pacemaker{executor}.Unmigrate(context.Background())

			if err != nil {
				assert.EqualValues(t, tc.err.Error(), err.Error())
			}
			executor.AssertExpectations(t)
		})
	}
}
