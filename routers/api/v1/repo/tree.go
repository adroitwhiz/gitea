// Copyright 2018 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"net/http"

	"code.gitea.io/gitea/modules/context"
	"code.gitea.io/gitea/modules/repofiles"
	api "code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/modules/web"
)

// GetTree get the tree of a repository.
func GetTree(ctx *context.APIContext) {
	// swagger:operation GET /repos/{owner}/{repo}/git/trees/{sha} repository GetTree
	// ---
	// summary: Gets the tree of a repository.
	// produces:
	// - application/json
	// parameters:
	// - name: owner
	//   in: path
	//   description: owner of the repo
	//   type: string
	//   required: true
	// - name: repo
	//   in: path
	//   description: name of the repo
	//   type: string
	//   required: true
	// - name: sha
	//   in: path
	//   description: sha of the commit
	//   type: string
	//   required: true
	// - name: recursive
	//   in: query
	//   description: show all directories and files
	//   required: false
	//   type: boolean
	// - name: page
	//   in: query
	//   description: page number; the 'truncated' field in the response will be true if there are still more items after this page, false if the last page
	//   required: false
	//   type: integer
	// - name: per_page
	//   in: query
	//   description: number of items per page
	//   required: false
	//   type: integer
	// responses:
	//   "200":
	//     "$ref": "#/responses/GitTreeResponse"
	//   "400":
	//     "$ref": "#/responses/error"

	sha := ctx.Params(":sha")
	if len(sha) == 0 {
		ctx.Error(http.StatusBadRequest, "", "sha not provided")
		return
	}
	if tree, err := repofiles.GetTreeBySHA(ctx.Repo.Repository, sha, ctx.FormInt("page"), ctx.FormInt("per_page"), ctx.FormBool("recursive")); err != nil {
		ctx.Error(http.StatusBadRequest, "", err.Error())
	} else {
		ctx.JSON(http.StatusOK, tree)
	}
}

// WriteTree write a tree to a repository.
func WriteTree(ctx *context.APIContext) {
	// swagger:operation POST /repos/{owner}/{repo}/git/trees repository WriteTree
	// ---
	// summary: Writes a tree to a repository.
	// produces:
	// - application/json
	// parameters:
	// - name: owner
	//   in: path
	//   description: owner of the repo
	//   type: string
	//   required: true
	// - name: repo
	//   in: path
	//   description: name of the repo
	//   type: string
	//   required: true
	// - in: body
	//   name: body
	//   required: true
	//   schema:
	//     "$ref": "#/definitions/GitWriteTreeOptions"
	// responses:
	//   "200":
	//     "$ref": "#/responses/GitWriteTreeResponse"
	//   "400":
	//     "$ref": "#/responses/error"
	//   "403":
	//     "$ref": "#/responses/error"

	// TODO: move this into reqRepoWriter
	if ctx.Repo.Repository.IsMirror || ctx.Repo.Repository.IsArchived {
		ctx.Error(http.StatusForbidden, "Repository is archived or a mirror", nil)
		return
	}

	apiOpts := web.GetForm(ctx).(*api.GitWriteTreeOptions)

	if sha, err := repofiles.WriteTree(ctx.Repo.Repository, apiOpts.Tree, apiOpts.BaseTree); err != nil {
		ctx.Error(http.StatusBadRequest, "", err)
	} else {
		ctx.JSON(http.StatusCreated, sha)
	}
}
