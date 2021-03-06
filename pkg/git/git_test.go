/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package git_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"k8s.io/release/pkg/git"
	"k8s.io/release/pkg/git/gitfakes"
)

func newSUT() (*git.Repo, *gitfakes.FakeWorktree) {
	repoMock := &gitfakes.FakeRepository{}
	worktreeMock := &gitfakes.FakeWorktree{}

	repo := &git.Repo{}
	repo.SetWorktree(worktreeMock)
	repo.SetInnerRepo(repoMock)

	return repo, worktreeMock
}

func TestCommit(t *testing.T) {
	repo, worktreeMock := newSUT()
	require.Nil(t, repo.Commit("msg"))
	require.Equal(t, worktreeMock.CommitCallCount(), 1)
}

func TestGetDefaultKubernetesRepoURLSuccess(t *testing.T) {
	testcases := []struct {
		name     string
		org      string
		useSSH   bool
		expected string
	}{
		{
			name:     "default HTTPS",
			expected: "https://github.com/kubernetes/kubernetes",
		},
	}

	for _, tc := range testcases {
		t.Logf("Test case: %s", tc.name)

		actual := git.GetDefaultKubernetesRepoURL()
		require.Equal(t, tc.expected, actual)
	}
}

// createTestRepository creates a test repo, cd into it and returns the path
func createTestRepository() (repoPath string, err error) {
	repoPath, err = ioutil.TempDir(os.TempDir(), "sigrelease-test-repo-*")
	if err != nil {
		return "", errors.Wrap(err, "creating a directory for test repository")
	}
	if err := os.Chdir(repoPath); err != nil {
		return "", errors.Wrap(err, "cd'ing into test repository")
	}
	out, err := exec.Command("git", "init").Output()
	if err != nil {
		return "", errors.Wrapf(err, "initializing test repository: %s", out)
	}
	return repoPath, nil
}

func TestGetUserName(t *testing.T) {
	const fakeUserName = "SIG Release Test User"
	currentDir, err := os.Getwd()
	require.Nil(t, err, "error reading the current directory")
	defer os.Chdir(currentDir) // nolint: errcheck

	// Create an empty repo and configure the users name to test
	repoPath, err := createTestRepository()
	require.Nil(t, err, "getting a test repo")

	// Call git to configure the user's name:
	_, err = exec.Command("git", "config", "user.name", fakeUserName).Output()
	require.Nil(t, err, fmt.Sprintf("configuring fake user email in %s", repoPath))

	testRepo, err := git.OpenRepo(repoPath)
	require.Nil(t, err, fmt.Sprintf("opening test repo in %s", repoPath))
	defer testRepo.Cleanup() // nolint: errcheck

	actual, err := git.GetUserName()
	require.Nil(t, err)
	require.Equal(t, fakeUserName, actual)
	require.NotEqual(t, fakeUserName, "")
}

func TestGetUserEmail(t *testing.T) {
	const fakeUserEmail = "kubernetes-test@example.com"
	currentDir, err := os.Getwd() // nolint: errcheck
	require.Nil(t, err, "error reading the current directory")
	defer os.Chdir(currentDir) // nolint: errcheck

	// Create an empty repo and configure the users name to test
	repoPath, err := createTestRepository()
	require.Nil(t, err, "getting a test repo")

	// Call git to configure the user's name:
	_, err = exec.Command("git", "config", "user.email", fakeUserEmail).Output()
	require.Nil(t, err, fmt.Sprintf("configuring fake user email in %s", repoPath))

	testRepo, err := git.OpenRepo(repoPath)
	require.Nil(t, err, fmt.Sprintf("opening test repo in %s", repoPath))
	defer testRepo.Cleanup() // nolint: errcheck

	// Do the actual call
	actual, err := git.GetUserEmail()
	require.Nil(t, err)
	require.Equal(t, fakeUserEmail, actual)
	require.NotEqual(t, fakeUserEmail, "")
}

func TestGetKubernetesRepoURLSuccess(t *testing.T) {
	testcases := []struct {
		name     string
		org      string
		useSSH   bool
		expected string
	}{
		{
			name:     "default HTTPS",
			expected: "https://github.com/kubernetes/kubernetes",
		},
		{
			name:     "ssh with custom org",
			org:      "fake-org",
			useSSH:   true,
			expected: "git@github.com:fake-org/kubernetes",
		},
	}

	for _, tc := range testcases {
		t.Logf("Test case: %s", tc.name)

		actual := git.GetKubernetesRepoURL(tc.org, tc.useSSH)
		require.Equal(t, tc.expected, actual)
	}
}

func TestGetRepoURLSuccess(t *testing.T) {
	testcases := []struct {
		name     string
		org      string
		repo     string
		useSSH   bool
		expected string
	}{
		{
			name:     "default Kubernetes HTTPS",
			org:      "kubernetes",
			repo:     "kubernetes",
			expected: "https://github.com/kubernetes/kubernetes",
		},
		{
			name:     "ssh with custom org",
			org:      "fake-org",
			repo:     "repofoo",
			useSSH:   true,
			expected: "git@github.com:fake-org/repofoo",
		},
	}

	for _, tc := range testcases {
		t.Logf("Test case: %s", tc.name)

		actual := git.GetRepoURL(tc.org, tc.repo, tc.useSSH)
		require.Equal(t, tc.expected, actual)
	}
}

func TestRemotify(t *testing.T) {
	testcases := []struct{ provided, expected string }{
		{provided: git.DefaultBranch, expected: git.DefaultRemote + "/" + git.DefaultBranch},
		{provided: "origin/ref", expected: "origin/ref"},
		{provided: "base/another_ref", expected: "base/another_ref"},
	}

	for _, tc := range testcases {
		require.Equal(t, git.Remotify(tc.provided), tc.expected)
	}
}

func TestIsDirtyMockSuccess(t *testing.T) {
	repo, _ := newSUT()

	dirty, err := repo.IsDirty()

	require.Nil(t, err)
	require.False(t, dirty)
}

func TestIsDirtyMockSuccessDirty(t *testing.T) {
	repo, worktreeMock := newSUT()
	worktreeMock.StatusReturns(gogit.Status{
		"file": &gogit.FileStatus{
			Worktree: gogit.Modified,
		},
	}, nil)

	dirty, err := repo.IsDirty()

	require.Nil(t, err)
	require.True(t, dirty)
}

func TestIsDirtyMockFailureWorktreeStatus(t *testing.T) {
	repo, worktreeMock := newSUT()
	worktreeMock.StatusReturns(gogit.Status{}, errors.New(""))

	dirty, err := repo.IsDirty()

	require.NotNil(t, err)
	require.False(t, dirty)
}
