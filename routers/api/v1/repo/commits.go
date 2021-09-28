// Copyright 2018 The Gogs Authors. All rights reserved.
// Copyright 2019 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"math"
	"net/http"
	"strconv"

	"code.gitea.io/gitea/models"
	"code.gitea.io/gitea/modules/context"
	"code.gitea.io/gitea/modules/convert"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/repofiles"
	"code.gitea.io/gitea/modules/setting"
	api "code.gitea.io/gitea/modules/structs"
	"code.gitea.io/gitea/modules/validation"
	"code.gitea.io/gitea/modules/web"
	"code.gitea.io/gitea/routers/api/v1/utils"
)

// GetSingleCommit get a commit via sha
func GetSingleCommit(ctx *context.APIContext) {
	// swagger:operation GET /repos/{owner}/{repo}/git/commits/{sha} repository repoGetSingleCommit
	// ---
	// summary: Get a single commit from a repository
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
	//   description: a git ref or commit sha
	//   type: string
	//   required: true
	// responses:
	//   "200":
	//     "$ref": "#/responses/Commit"
	//   "422":
	//     "$ref": "#/responses/validationError"
	//   "404":
	//     "$ref": "#/responses/notFound"

	sha := ctx.Params(":sha")
	if (validation.GitRefNamePatternInvalid.MatchString(sha) || !validation.CheckGitRefAdditionalRulesValid(sha)) && !git.SHAPattern.MatchString(sha) {
		ctx.Error(http.StatusUnprocessableEntity, "no valid ref or sha", fmt.Sprintf("no valid ref or sha: %s", sha))
		return
	}
	getCommit(ctx, sha)
}

func getCommit(ctx *context.APIContext, identifier string) {
	gitRepo, err := git.OpenRepository(ctx.Repo.Repository.RepoPath())
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "OpenRepository", err)
		return
	}
	defer gitRepo.Close()
	commit, err := gitRepo.GetCommit(identifier)
	if err != nil {
		if git.IsErrNotExist(err) {
			ctx.NotFound(identifier)
			return
		}
		ctx.Error(http.StatusInternalServerError, "gitRepo.GetCommit", err)
		return
	}

	json, err := convert.ToCommit(ctx.Repo.Repository, commit, nil)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "toCommit", err)
		return
	}
	ctx.JSON(http.StatusOK, json)
}

// GetAllCommits get all commits via
func GetAllCommits(ctx *context.APIContext) {
	// swagger:operation GET /repos/{owner}/{repo}/commits repository repoGetAllCommits
	// ---
	// summary: Get a list of all commits from a repository
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
	//   in: query
	//   description: SHA or branch to start listing commits from (usually 'master')
	//   type: string
	// - name: page
	//   in: query
	//   description: page number of results to return (1-based)
	//   type: integer
	// - name: limit
	//   in: query
	//   description: page size of results
	//   type: integer
	// responses:
	//   "200":
	//     "$ref": "#/responses/CommitList"
	//   "404":
	//     "$ref": "#/responses/notFound"
	//   "409":
	//     "$ref": "#/responses/EmptyRepository"

	if ctx.Repo.Repository.IsEmpty {
		ctx.JSON(http.StatusConflict, api.APIError{
			Message: "Git Repository is empty.",
			URL:     setting.API.SwaggerURL,
		})
		return
	}

	gitRepo, err := git.OpenRepository(ctx.Repo.Repository.RepoPath())
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "OpenRepository", err)
		return
	}
	defer gitRepo.Close()

	listOptions := utils.GetListOptions(ctx)
	if listOptions.Page <= 0 {
		listOptions.Page = 1
	}

	if listOptions.PageSize > setting.Git.CommitsRangeSize {
		listOptions.PageSize = setting.Git.CommitsRangeSize
	}

	sha := ctx.FormString("sha")

	var baseCommit *git.Commit
	if len(sha) == 0 {
		// no sha supplied - use default branch
		head, err := gitRepo.GetHEADBranch()
		if err != nil {
			ctx.Error(http.StatusInternalServerError, "GetHEADBranch", err)
			return
		}

		baseCommit, err = gitRepo.GetBranchCommit(head.Name)
		if err != nil {
			ctx.Error(http.StatusInternalServerError, "GetCommit", err)
			return
		}
	} else {
		// get commit specified by sha
		baseCommit, err = gitRepo.GetCommit(sha)
		if err != nil {
			ctx.Error(http.StatusInternalServerError, "GetCommit", err)
			return
		}
	}

	// Total commit count
	commitsCountTotal, err := baseCommit.CommitsCount()
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "GetCommitsCount", err)
		return
	}

	pageCount := int(math.Ceil(float64(commitsCountTotal) / float64(listOptions.PageSize)))

	// Query commits
	commits, err := baseCommit.CommitsByRange(listOptions.Page, listOptions.PageSize)
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "CommitsByRange", err)
		return
	}

	userCache := make(map[string]*models.User)

	apiCommits := make([]*api.Commit, len(commits))
	for i, commit := range commits {
		// Create json struct
		apiCommits[i], err = convert.ToCommit(ctx.Repo.Repository, commit, userCache)
		if err != nil {
			ctx.Error(http.StatusInternalServerError, "toCommit", err)
			return
		}
	}

	ctx.SetLinkHeader(int(commitsCountTotal), listOptions.PageSize)
	ctx.SetTotalCountHeader(commitsCountTotal)

	// kept for backwards compatibility
	ctx.Header().Set("X-Page", strconv.Itoa(listOptions.Page))
	ctx.Header().Set("X-PerPage", strconv.Itoa(listOptions.PageSize))
	ctx.Header().Set("X-Total", strconv.FormatInt(commitsCountTotal, 10))
	ctx.Header().Set("X-PageCount", strconv.Itoa(pageCount))
	ctx.Header().Set("X-HasMore", strconv.FormatBool(listOptions.Page < pageCount))
	ctx.AppendAccessControlExposeHeaders("X-Page", "X-PerPage", "X-Total", "X-PageCount", "X-HasMore")

	ctx.JSON(http.StatusOK, &apiCommits)
}

// DownloadCommitDiffOrPatch render a commit's raw diff or patch
func DownloadCommitDiffOrPatch(ctx *context.APIContext) {
	// swagger:operation GET /repos/{owner}/{repo}/git/commits/{sha}.{diffType} repository repoDownloadCommitDiffOrPatch
	// ---
	// summary: Get a commit's diff or patch
	// produces:
	// - text/plain
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
	//   description: SHA of the commit to get
	//   type: string
	//   required: true
	// - name: diffType
	//   in: path
	//   description: whether the output is diff or patch
	//   type: string
	//   enum: [diff, patch]
	//   required: true
	// responses:
	//   "200":
	//     "$ref": "#/responses/string"
	//   "404":
	//     "$ref": "#/responses/notFound"
	repoPath := models.RepoPath(ctx.Repo.Owner.Name, ctx.Repo.Repository.Name)
	if err := git.GetRawDiff(
		repoPath,
		ctx.Params(":sha"),
		git.RawDiffType(ctx.Params(":diffType")),
		ctx.Resp,
	); err != nil {
		if git.IsErrNotExist(err) {
			ctx.NotFound(ctx.Params(":sha"))
			return
		}
		ctx.Error(http.StatusInternalServerError, "DownloadCommitDiffOrPatch", err)
		return
	}
}

// CreateCommit creates a new git commit object
func CreateCommit(ctx *context.APIContext) {
	// swagger:operation POST /repos/{owner}/{repo}/git/commits repository CreateCommit
	// ---
	// summary: Create a new commit object
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
	//     "$ref": "#/definitions/CreateCommitOptions"
	// responses:
	//   "200":
	//     "$ref": "#/responses/CreateCommitResponse"
	//   "400":
	//     "$ref": "#/responses/error"
	//   "403":
	//     "$ref": "#/responses/error"

	// TODO: move this into reqRepoWriter
	if ctx.Repo.Repository.IsMirror || ctx.Repo.Repository.IsArchived {
		ctx.Error(http.StatusForbidden, "Repository is archived or a mirror", nil)
		return
	}

	gitRepo, err := git.OpenRepository(ctx.Repo.Repository.RepoPath())
	if err != nil {
		ctx.Error(http.StatusInternalServerError, "CreateCommit", err)
	}

	apiOpts := web.GetForm(ctx).(*api.CreateCommitOptions)

	authorOpts := &repofiles.IdentityOptions{
		Name:  apiOpts.Author.Name,
		Email: apiOpts.Author.Email,
	}

	committerOpts := &repofiles.IdentityOptions{
		Name:  apiOpts.Committer.Name,
		Email: apiOpts.Committer.Email,
	}

	dateOpts := &repofiles.CommitDateOptions{
		Author:    apiOpts.Dates.Author,
		Committer: apiOpts.Dates.Committer,
	}

	author, committer := repofiles.GetAuthorAndCommitterUsers(authorOpts, committerOpts, ctx.User)

	// CommitTree allows for any Git ref to be passed as parents. Limit it to SHA hashes.
	for _, parent := range *apiOpts.Parents {
		_, err := git.NewIDFromString(parent)
		if err != nil {
			ctx.Error(http.StatusBadRequest, fmt.Sprintf("Invalid SHA hash: %s", parent), err)
			return
		}
	}

	if shaString, verification, err := repofiles.CommitTree(ctx.Repo.Repository, gitRepo, author, committer, apiOpts.Tree, apiOpts.Message, apiOpts.Signoff, repofiles.CommitTreeOptions{
		Parents: apiOpts.Parents,
		Dates:   dateOpts,
	}); err != nil {
		ctx.Error(http.StatusBadRequest, "", err)
	} else {
		ctx.JSON(http.StatusCreated, api.CreateCommitResponse{
			URL:          ctx.Repo.Repository.APIURL() + "/git/commits/" + shaString,
			SHA:          shaString,
			Verification: verification,
		})
	}
}
