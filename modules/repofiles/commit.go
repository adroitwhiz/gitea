// Copyright 2019 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repofiles

import (
	"fmt"

	"code.gitea.io/gitea/models"
	"code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/log"
)

// CommitTreeOptions represents the non-mandatory options to CommitTree
type CommitTreeOptions struct {
	Parents []string
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
func CommitTree(repo *models.Repository, gitRepo *git.Repository, author, committer *models.User, treeHash string, message string, signoff bool, opts CommitTreeOptions) (string, error) {
	// Initialize default parent. Dates will be set in git.CommitTree if not provided here.
	if len(opts.Parents) == 0 {
		opts.Parents = append(opts.Parents, "HEAD")
	}

	authorSig := author.NewGitSig()
	committerSig := committer.NewGitSig()

	err := git.LoadGitVersion()
	if err != nil {
		return "", fmt.Errorf("unable to get git version: %v", err)
	}

	treeID, err := git.NewIDFromString(treeHash)
	if err != nil {
		return "", err
	}

	gitOpts := git.CommitTreeOpts{
		Parents:  []string{"HEAD"},
		Message:  message,
		Trailers: make(map[string]string),
	}

	if opts.Dates != nil {
		gitOpts.AuthorDate = opts.Dates.Author
		gitOpts.CommitterDate = opts.Dates.Committer
	}

	// Determine if we should sign
	if git.CheckGitVersionAtLeast("1.7.9") == nil {
		sign, keyID, signer, _ := repo.SignCRUDAction(author, gitRepo.Path, opts.Parents)
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
		return "", err
	}
	return commitID.String(), nil
}
