package packager

import (
	"errors"
	"fmt"
	"testing"

	"github.com/defenseunicorns/zarf/src/config"
	"github.com/defenseunicorns/zarf/src/config/lang"
	"github.com/defenseunicorns/zarf/src/internal/cluster"
	"github.com/defenseunicorns/zarf/src/pkg/k8s"
	"github.com/defenseunicorns/zarf/src/types"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8sTesting "k8s.io/client-go/testing"
)

// TestValidatePackageArchitecture verifies that Zarf validates package architecture against cluster architecture correctly.
func TestValidatePackageArchitecture(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name          string
		pkgArch       string
		clusterArch   string
		expectedError error
		getArchError  error
	}

	testCases := []testCase{
		{
			name:          "architecture match",
			pkgArch:       "amd64",
			clusterArch:   "amd64",
			expectedError: nil,
		},
		{
			name:          "architecture mismatch",
			pkgArch:       "arm64",
			clusterArch:   "amd64",
			expectedError: fmt.Errorf(lang.CmdPackageDeployValidateArchitectureErr, "arm64", "amd64"),
		},
		{
			name:          "ignore validation when package arch equals 'multi'",
			pkgArch:       "multi",
			clusterArch:   "not evaluated",
			expectedError: nil,
		},
		{
			name:          "test the error path when fetching cluster architecture fails",
			pkgArch:       "amd64",
			getArchError:  errors.New("error fetching cluster architecture"),
			expectedError: lang.ErrUnableToCheckArch,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			mockClient := fake.NewSimpleClientset()
			logger := func(string, ...interface{}) {}

			// Create a Packager instance with package architecture set and a mock Kubernetes client.
			p := &Packager{
				arch: testCase.pkgArch,
				cluster: &cluster.Cluster{
					K8s: &k8s.K8s{
						Clientset: mockClient,
						Log:       logger,
					},
				},
			}

			// Set up test data for fetching cluster architecture.
			mockClient.Fake.PrependReactor("list", "nodes", func(action k8sTesting.Action) (handled bool, ret runtime.Object, err error) {
				// Return an error for cases that test this error path.
				if testCase.getArchError != nil {
					return true, nil, testCase.getArchError
				}

				// Create a NodeList object to fetch cluster architecture with the mock client.
				nodeList := &v1.NodeList{
					Items: []v1.Node{
						{
							Status: v1.NodeStatus{
								NodeInfo: v1.NodeSystemInfo{
									Architecture: testCase.clusterArch,
								},
							},
						},
					},
				}
				return true, nodeList, nil
			})

			err := p.validatePackageArchitecture()

			require.Equal(t, testCase.expectedError, err)
		})
	}
}

// TestValidateLastNonBreakingVersion verifies that Zarf validates the lastNonBreakingVersion of packages against the CLI version correctly.
func TestValidateLastNonBreakingVersion(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name                   string
		cliVersion             string
		lastNonBreakingVersion string
		expectedErrorMessage   string
		expectedWarningMessage string
		returnError            bool
		throwWarning           bool
	}

	testCases := []testCase{
		{
			name:                   "CLI version less than lastNonBreakingVersion",
			cliVersion:             "v0.26.4",
			lastNonBreakingVersion: "v0.27.0",
			returnError:            false,
			throwWarning:           true,
			expectedWarningMessage: fmt.Sprintf(
				lang.CmdPackageDeployValidateLastNonBreakingVersionWarn,
				"v0.26.4",
				"v0.27.0",
				"v0.27.0",
			),
		},
		{
			name:                   "invalid semantic version (CLI version)",
			cliVersion:             "invalidSemanticVersion",
			lastNonBreakingVersion: "v0.0.1",
			returnError:            false,
			throwWarning:           true,
			expectedWarningMessage: fmt.Sprintf(lang.CmdPackageDeployInvalidCLIVersionWarn, "invalidSemanticVersion"),
		},
		{
			name:                   "invalid semantic version (lastNonBreakingVersion)",
			cliVersion:             "v0.0.1",
			lastNonBreakingVersion: "invalidSemanticVersion",
			throwWarning:           false,
			returnError:            true,
			expectedErrorMessage:   "unable to parse lastNonBreakingVersion",
		},
		{
			name:                   "CLI version greater than lastNonBreakingVersion",
			cliVersion:             "v0.28.2",
			lastNonBreakingVersion: "v0.27.0",
			returnError:            false,
			throwWarning:           false,
		},
		{
			name:                   "CLI version equal to lastNonBreakingVersion",
			cliVersion:             "v0.27.0",
			lastNonBreakingVersion: "v0.27.0",
			returnError:            false,
			throwWarning:           false,
		},
		{
			name:                   "empty lastNonBreakingVersion",
			cliVersion:             "this shouldn't get evaluated when the lastNonBreakingVersion is empty",
			lastNonBreakingVersion: "",
			returnError:            false,
			throwWarning:           false,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			config.CLIVersion = testCase.cliVersion

			p := &Packager{
				cfg: &types.PackagerConfig{
					Pkg: types.ZarfPackage{
						Build: types.ZarfBuildData{
							LastNonBreakingVersion: testCase.lastNonBreakingVersion,
						},
					},
				},
			}

			err := p.validateLastNonBreakingVersion()

			switch {
			case testCase.returnError:
				require.ErrorContains(t, err, testCase.expectedErrorMessage)
				require.Empty(t, p.warnings, "Expected no warnings for test case: %s", testCase.name)
			case testCase.throwWarning:
				require.Contains(t, p.warnings, testCase.expectedWarningMessage)
				require.NoError(t, err, "Expected no error for test case: %s", testCase.name)
			default:
				require.NoError(t, err, "Expected no error for test case: %s", testCase.name)
				require.Empty(t, p.warnings, "Expected no warnings for test case: %s", testCase.name)
			}
		})
	}
}
