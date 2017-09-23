package pacemaker

import (
	"errors"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type fakeExecutor struct{ mock.Mock }

func (e fakeExecutor) CombinedOutput(name string, arg ...string) ([]byte, error) {
	args := e.Called(name, arg)
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
			executor.On("CombinedOutput", "cibadmin", []string{"--query", "--local"}).Return(fixture, nil)

			nodes, err := Cib{executor}.Get(MasterXPath)

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
