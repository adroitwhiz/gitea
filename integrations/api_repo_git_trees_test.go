// Copyright 2019 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package integrations

import (
	"fmt"
	"net/http"
	"testing"

	"code.gitea.io/gitea/models"
	"code.gitea.io/gitea/models/db"
	api "code.gitea.io/gitea/modules/structs"
	"github.com/stretchr/testify/assert"
)

func TestAPIReposGitTrees(t *testing.T) {
	defer prepareTestEnv(t)()
	user2 := db.AssertExistsAndLoadBean(t, &models.User{ID: 2}).(*models.User)               // owner of the repo1 & repo16
	user3 := db.AssertExistsAndLoadBean(t, &models.User{ID: 3}).(*models.User)               // owner of the repo3
	user4 := db.AssertExistsAndLoadBean(t, &models.User{ID: 4}).(*models.User)               // owner of neither repos
	repo1 := db.AssertExistsAndLoadBean(t, &models.Repository{ID: 1}).(*models.Repository)   // public repo
	repo3 := db.AssertExistsAndLoadBean(t, &models.Repository{ID: 3}).(*models.Repository)   // public repo
	repo16 := db.AssertExistsAndLoadBean(t, &models.Repository{ID: 16}).(*models.Repository) // private repo
	repo1TreeSHA := "65f1bf27bc3bf70f64657658635e66094edbcb4d"
	repo3TreeSHA := "2a47ca4b614a9f5a43abbd5ad851a54a616ffee6"
	repo16TreeSHA := "69554a64c1e6030f051e5c3f94bfbd773cd6a324"
	badSHA := "0000000000000000000000000000000000000000"

	// Login as User2.
	session := loginUser(t, user2.Name)
	token := getTokenForLoggedInUser(t, session)
	session = emptyTestSession(t) // don't want anyone logged in for this

	// Test a public repo that anyone can GET the tree of
	for _, ref := range [...]string{
		"master",     // Branch
		repo1TreeSHA, // Tree SHA
	} {
		req := NewRequestf(t, "GET", "/api/v1/repos/%s/%s/git/trees/%s", user2.Name, repo1.Name, ref)
		session.MakeRequest(t, req, http.StatusOK)
	}

	// Tests a private repo with no token so will fail
	for _, ref := range [...]string{
		"master",     // Branch
		repo1TreeSHA, // Tag
	} {
		req := NewRequestf(t, "GET", "/api/v1/repos/%s/%s/git/trees/%s", user2.Name, repo16.Name, ref)
		session.MakeRequest(t, req, http.StatusNotFound)
	}

	// Test using access token for a private repo that the user of the token owns
	req := NewRequestf(t, "GET", "/api/v1/repos/%s/%s/git/trees/%s?token=%s", user2.Name, repo16.Name, repo16TreeSHA, token)
	session.MakeRequest(t, req, http.StatusOK)

	// Test using bad sha
	req = NewRequestf(t, "GET", "/api/v1/repos/%s/%s/git/trees/%s", user2.Name, repo1.Name, badSHA)
	session.MakeRequest(t, req, http.StatusBadRequest)

	// Test using org repo "user3/repo3" where user2 is a collaborator
	req = NewRequestf(t, "GET", "/api/v1/repos/%s/%s/git/trees/%s?token=%s", user3.Name, repo3.Name, repo3TreeSHA, token)
	session.MakeRequest(t, req, http.StatusOK)

	// Test using org repo "user3/repo3" with no user token
	req = NewRequestf(t, "GET", "/api/v1/repos/%s/%s/git/trees/%s", user3.Name, repo3TreeSHA, repo3.Name)
	session.MakeRequest(t, req, http.StatusNotFound)

	// Login as User4.
	session = loginUser(t, user4.Name)
	token4 := getTokenForLoggedInUser(t, session)
	session = emptyTestSession(t) // don't want anyone logged in for this

	// Test using org repo "user3/repo3" where user4 is a NOT collaborator
	req = NewRequestf(t, "GET", "/api/v1/repos/%s/%s/git/trees/d56a3073c1dbb7b15963110a049d50cdb5db99fc?access=%s", user3.Name, repo3.Name, token4)
	session.MakeRequest(t, req, http.StatusNotFound)

	// Test writing trees

	// write trees with file content
	writeTreeOptions := api.GitWriteTreeOptions{
		Tree: []*api.GitWriteTreeEntry{
			{
				Name:    "file1",
				Mode:    "100644",
				Content: "dGVzdCBjb250ZW50cwo=",
			},
			{
				Name:    "file2",
				Mode:    "100644",
				Content: "dGVzdCBjb250ZW50cyAyCg==",
			},
			{
				Name:    "file3",
				Mode:    "100755",
				Content: "dGVzdCBjb250ZW50cyAzCg==",
			},
		},
	}

	req = NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/git/trees?token=%s", user2.Name, repo1.Name, token), &writeTreeOptions)
	resp := session.MakeRequest(t, req, http.StatusCreated)
	var writeTreeResponse api.GitWriteTreeResponse
	DecodeJSON(t, resp, &writeTreeResponse)
	assert.EqualValues(t, "db8bcff0372fbe624f9e01d6ae8d06e86775f728", writeTreeResponse.SHA)

	// write trees with references
	writeTreeOptions = api.GitWriteTreeOptions{
		Tree: []*api.GitWriteTreeEntry{
			{
				Name: "subtree",
				Mode: "040000",
				// this references the tree we just uploaded above
				SHA: "db8bcff0372fbe624f9e01d6ae8d06e86775f728",
			},
			{
				Name: "file2",
				Mode: "100644",
				// the SHA of file2 uploaded above
				SHA: "1c658e12ca5fd1a0bd1e0ed930c216ed937ff919",
			},
		},
	}

	req = NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/git/trees?token=%s", user2.Name, repo1.Name, token), &writeTreeOptions)
	resp = session.MakeRequest(t, req, http.StatusCreated)
	DecodeJSON(t, resp, &writeTreeResponse)
	assert.EqualValues(t, "0c58f5ef6fe05eb79f8cac8f3aca29f5cf89316b", writeTreeResponse.SHA)

	// overwrite entries from base tree
	writeTreeOptions = api.GitWriteTreeOptions{
		BaseTree: "db8bcff0372fbe624f9e01d6ae8d06e86775f728",
		Tree: []*api.GitWriteTreeEntry{
			{
				// remove file2 by providing no SHA or content
				Name: "file2",
				Mode: "100644",
			},
			{
				Name:    "file3",
				Mode:    "100644",
				Content: "cmVwbGFjZW1lbnQgY29udGVudAo=",
			},
		},
	}

	req = NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/git/trees?token=%s", user2.Name, repo1.Name, token), &writeTreeOptions)
	resp = session.MakeRequest(t, req, http.StatusCreated)
	DecodeJSON(t, resp, &writeTreeResponse)
	assert.EqualValues(t, "c30b0114b5466e32610af85cd95bc59c226a7f2d", writeTreeResponse.SHA)

	// non-blob with content
	writeTreeOptions = api.GitWriteTreeOptions{
		Tree: []*api.GitWriteTreeEntry{
			{
				Name:    "dir",
				Mode:    "040000",
				Content: "Y29udGVudAo=",
			},
		},
	}

	req = NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/git/trees?token=%s", user2.Name, repo1.Name, token), &writeTreeOptions)
	session.MakeRequest(t, req, http.StatusBadRequest)

	// filename containing slash
	writeTreeOptions = api.GitWriteTreeOptions{
		Tree: []*api.GitWriteTreeEntry{
			{
				Name:    "sl/ash",
				Mode:    "100644",
				Content: "Y29udGVudAo=",
			},
		},
	}

	req = NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/git/trees?token=%s", user2.Name, repo1.Name, token), &writeTreeOptions)
	session.MakeRequest(t, req, http.StatusBadRequest)

	// file named .git
	writeTreeOptions = api.GitWriteTreeOptions{
		Tree: []*api.GitWriteTreeEntry{
			{
				Name:    ".git",
				Mode:    "100644",
				Content: "Y29udGVudAo=",
			},
		},
	}

	req = NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/git/trees?token=%s", user2.Name, repo1.Name, token), &writeTreeOptions)
	session.MakeRequest(t, req, http.StatusBadRequest)

	// not-quite-valid-file mode
	writeTreeOptions = api.GitWriteTreeOptions{
		Tree: []*api.GitWriteTreeEntry{
			{
				Name:    "file",
				Mode:    "0100644",
				Content: "Y29udGVudAo=",
			},
		},
	}

	req = NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/git/trees?token=%s", user2.Name, repo1.Name, token), &writeTreeOptions)
	session.MakeRequest(t, req, http.StatusUnprocessableEntity)

	// invalid sha
	writeTreeOptions = api.GitWriteTreeOptions{
		Tree: []*api.GitWriteTreeEntry{
			{
				Name: "file",
				Mode: "100644",
				SHA:  "not a hash",
			},
		},
	}

	req = NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/git/trees?token=%s", user2.Name, repo1.Name, token), &writeTreeOptions)
	session.MakeRequest(t, req, http.StatusBadRequest)

	// sha referencing nonexistent object
	writeTreeOptions = api.GitWriteTreeOptions{
		Tree: []*api.GitWriteTreeEntry{
			{
				Name: "file",
				Mode: "100644",
				SHA:  badSHA,
			},
		},
	}

	req = NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/git/trees?token=%s", user2.Name, repo1.Name, token), &writeTreeOptions)
	session.MakeRequest(t, req, http.StatusBadRequest)

	// sha and content both provided
	writeTreeOptions = api.GitWriteTreeOptions{
		Tree: []*api.GitWriteTreeEntry{
			{
				Name: "file",
				Mode: "100644",
				// the SHA of file2 uploaded above
				SHA:     "1c658e12ca5fd1a0bd1e0ed930c216ed937ff919",
				Content: "Y29udGVudAo=",
			},
		},
	}

	req = NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/git/trees?token=%s", user2.Name, repo1.Name, token), &writeTreeOptions)
	session.MakeRequest(t, req, http.StatusBadRequest)
}
