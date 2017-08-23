package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/urfave/cli"
)

type assertFlagsTestSuite struct {
	suite.Suite
	app *cli.App
}

func (suite *assertFlagsTestSuite) SetupTest() {
	suite.app = cli.NewApp()
	suite.app.Flags = []cli.Flag{cli.StringFlag{Name: "global"}}
	suite.app.Commands = []cli.Command{
		{
			Name:   "command",
			Flags:  []cli.Flag{cli.StringFlag{Name: "local"}},
			Action: checkMissingFlags,
		},
	}
}

func (suite *assertFlagsTestSuite) TestSuccessWithAllFlags() {
	err := suite.app.Run([]string{"", "--global", "g", "command", "--local", "l"})
	assert.Nil(suite.T(), err, "should not return error")
}

func (suite *assertFlagsTestSuite) TestErrorWhenMissingGlobal() {
	err := suite.app.Run([]string{"", "command", "--local", "l"})
	if assert.Error(suite.T(), err, "did not return an error") {
		assert.Regexp(suite.T(), "Missing configuration flags.*global", err.Error())
	}
}

func (suite *assertFlagsTestSuite) TestErrorWhenMissingLocal() {
	err := suite.app.Run([]string{"", "--global", "g", "command"})
	if assert.Error(suite.T(), err, "did not return an error") {
		assert.Regexp(suite.T(), "Missing configuration flags.*local", err.Error())
	}
}

func TestAssertFlagsTestSuite(t *testing.T) {
	suite.Run(t, new(assertFlagsTestSuite))
}
