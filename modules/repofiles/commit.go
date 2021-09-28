// Copyright 2019 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repofiles

import (
	"fmt"
	"strings"
	"time"

	"code.gitea.io/gitea/models"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/structs"
)

// IdentityOptions for a person's identity like an author or committer
type IdentityOptions struct {
	Name  string
	Email string
}

// CommitDateOptions store dates for GIT_AUTHOR_DATE and GIT_COMMITTER_DATE
type CommitDateOptions struct {
	Author    time.Time
	Committer time.Time
}

// CommitTreeOptions represents the non-mandatory options to CommitTree
type CommitTreeOptions struct {
	Parents *[]string
	Dates   *CommitDateOptions
}

// CountDivergingCommits determines how many commits a branch is ahead or behind the repository's base branch
func CountDivergingCommits(repo *models.Repository, branch string) (*git.DivergeObject, error) {
	divergence, err := git.GetDivergingCommits(repo.RepoPath(), repo.DefaultBranch, branch)
	if err != nil {
		return nil, err
	}
	return &divergence, nil
}

// CommitTree creates a commit from a given tree for the user with provided message.
// The Git repository and repository model passed need not be the same repository
// (for instance, a temporary upload repository has its own temporary Git repository.)
func CommitTree(repo *models.Repository, gitRepo *git.Repository, author, committer *models.User, treeHash string, message string, signoff bool, opts CommitTreeOptions) (string, *structs.PayloadCommitVerification, error) {
	// Initialize default parent. Dates will be set in git.CommitTree if not provided here.
	var parents []string
	if opts.Parents == nil {
		parents = []string{"HEAD"}
	} else {
		parents = *opts.Parents
	}

	authorSig := author.NewGitSig()
	committerSig := committer.NewGitSig()

	err := git.LoadGitVersion()
	if err != nil {
		return "", nil, fmt.Errorf("unable to get git version: %v", err)
	}

	treeID, err := git.NewIDFromString(treeHash)
	if err != nil {
		return "", nil, err
	}

	gitOpts := git.CommitTreeOpts{
		Parents:  parents,
		Message:  message,
		Trailers: make(map[string]string),
	}

	if opts.Dates != nil {
		gitOpts.AuthorDate = opts.Dates.Author
		gitOpts.CommitterDate = opts.Dates.Committer
	}

	// Determine if we should sign
	if git.CheckGitVersionAtLeast("1.7.9") == nil {
		sign, keyID, signer, _ := repo.SignCRUDAction(author, gitRepo.Path, parents)
		if sign {
			gitOpts.KeyID = keyID
			gitOpts.NoGPGSign = false
			gitOpts.AlwaysSign = true
			if repo.GetTrustModel() == models.CommitterTrustModel || repo.GetTrustModel() == models.CollaboratorCommitterTrustModel {
				if committerSig.Name != authorSig.Name || committerSig.Email != authorSig.Email {
					// Add trailers
					gitOpts.Trailers["Co-authored-by"] = committerSig.String()
					gitOpts.Trailers["Co-committed-by"] = committerSig.String()
				}
				committerSig = signer
			}
		} else if git.CheckGitVersionAtLeast("2.0.0") == nil {
			gitOpts.NoGPGSign = true
		}
	}

	if signoff {
		// Signed-off-by
		gitOpts.Trailers["Signed-off-by"] = committerSig.String()
	}

	commitID, err := gitRepo.CommitTree(authorSig, committerSig, treeID, gitOpts)
	if err != nil {
		log.Error("Unable to commit-tree in temporary repo: %s (%s) Error: %v",
			repo.FullName(), gitRepo.Path, err)
		return "", nil, err
	}
	commitIDString := commitID.String()

	commit, err := gitRepo.GetCommit(commitIDString)
	if err != nil {
		return "", nil, err
	}

	verification := GetPayloadCommitVerification(commit)

	return commitIDString, verification, nil
}

// GetAuthorAndCommitterUsers Gets the author and committer user objects from the IdentityOptions
func GetAuthorAndCommitterUsers(author, committer *IdentityOptions, doer *models.User) (authorUser, committerUser *models.User) {
	// Committer and author are optional. If they are not the doer (not same email address)
	// then we use bogus User objects for them to store their FullName and Email.
	// If only one of the two are provided, we set both of them to it.
	// If neither are provided, both are the doer.
	if committer != nil && committer.Email != "" {
		if doer != nil && strings.EqualFold(doer.Email, committer.Email) {
			committerUser = doer // the committer is the doer, so will use their user object
			if committer.Name != "" {
				committerUser.FullName = committer.Name
			}
		} else {
			committerUser = &models.User{
				FullName: committer.Name,
				Email:    committer.Email,
			}
		}
	}
	if author != nil && author.Email != "" {
		if doer != nil && strings.EqualFold(doer.Email, author.Email) {
			authorUser = doer // the author is the doer, so will use their user object
			if authorUser.Name != "" {
				authorUser.FullName = author.Name
			}
		} else {
			authorUser = &models.User{
				FullName: author.Name,
				Email:    author.Email,
			}
		}
	}
	if authorUser == nil {
		if committerUser != nil {
			authorUser = committerUser // No valid author was given so use the committer
		} else if doer != nil {
			authorUser = doer // No valid author was given and no valid committer so use the doer
		}
	}
	if committerUser == nil {
		committerUser = authorUser // No valid committer so use the author as the committer (was set to a valid user above)
	}
	return authorUser, committerUser
}
