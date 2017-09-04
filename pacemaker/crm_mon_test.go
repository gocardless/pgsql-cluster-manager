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
			"with three nodes when curly is master",
			"./fixtures/crm_mon_curly_master_three_node.xml",
			"curly",
			nil,
		},
		{
			"with two nodes when curly is master",
			"./fixtures/crm_mon_curly_master_two_node.xml",
			"curly",
			nil,
		},
		{
			"when we don't have quorum",
			"./fixtures/crm_mon_curly_master_three_node_no_quorum.xml",
			"",
			errors.New("Cannot find designated controller with quorum"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			executor := new(fakeExecutor)
			crmMon := CrmMon{executor}

			fixture, fixtureErr := ioutil.ReadFile(tc.fixture)
			require.Nil(t, fixtureErr)

			executor.On("CombinedOutput", "crm_mon", []string{"--as-xml"}).Return(fixture, nil)

			nodes, err := crmMon.Get("crm_mon/resources/resource[@id='PostgresqlVIP']/node[@name]")

			executor.AssertExpectations(t)

			if tc.err != nil {
				if assert.Error(t, err, "expected Get() to return error") {
					assert.Equal(t, tc.err.Error(), err.Error())
				}
			}

			if tc.value != "" {
				assert.Equal(t, 1, len(nodes), "expected only one node result")
				assert.Equal(t, tc.value, nodes[0].SelectAttrValue("name", ""))
			}
		})
	}
}
