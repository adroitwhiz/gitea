// Copyright 2018 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import "code.gitea.io/gitea/models/db"

// CommentList defines a list of comments
type CommentList []*Comment

func (comments CommentList) getPosterIDs() []int64 {
	posterIDs := make(map[int64]struct{}, len(comments))
	for _, comment := range comments {
		if _, ok := posterIDs[comment.PosterID]; !ok {
			posterIDs[comment.PosterID] = struct{}{}
		}
	}
	return keysInt64(posterIDs)
}

func (comments CommentList) loadPosters(e db.Engine) error {
	if len(comments) == 0 {
		return nil
	}

	posterIDs := comments.getPosterIDs()
	posterMaps := make(map[int64]*User, len(posterIDs))
	left := len(posterIDs)
	for left > 0 {
		limit := defaultMaxInSize
		if left < limit {
			limit = left
		}
		err := e.
			In("id", posterIDs[:limit]).
			Find(&posterMaps)
		if err != nil {
			return err
		}
		left -= limit
		posterIDs = posterIDs[limit:]
	}

	for _, comment := range comments {
		if comment.PosterID <= 0 {
			continue
		}
		var ok bool
		if comment.Poster, ok = posterMaps[comment.PosterID]; !ok {
			comment.Poster = NewGhostUser()
		}
	}
	return nil
}

func (comments CommentList) getCommentIDs() []int64 {
	ids := make([]int64, 0, len(comments))
	for _, comment := range comments {
		ids = append(ids, comment.ID)
	}
	return ids
}

func (comments CommentList) getLabelIDs() []int64 {
	ids := make(map[int64]struct{}, len(comments))
	for _, comment := range comments {
		if _, ok := ids[comment.LabelID]; !ok {
			ids[comment.LabelID] = struct{}{}
		}
	}
	return keysInt64(ids)
}

func (comments CommentList) loadLabels(e db.Engine) error {
	if len(comments) == 0 {
		return nil
	}

	labelIDs := comments.getLabelIDs()
	commentLabels := make(map[int64]*Label, len(labelIDs))
	left := len(labelIDs)
	for left > 0 {
		limit := defaultMaxInSize
		if left < limit {
			limit = left
		}
		rows, err := e.
			In("id", labelIDs[:limit]).
			Rows(new(Label))
		if err != nil {
			return err
		}

		for rows.Next() {
			var label Label
			err = rows.Scan(&label)
			if err != nil {
				_ = rows.Close()
				return err
			}
			commentLabels[label.ID] = &label
		}
		_ = rows.Close()
		left -= limit
		labelIDs = labelIDs[limit:]
	}

	for _, comment := range comments {
		comment.Label = commentLabels[comment.ID]
	}
	return nil
}

func (comments CommentList) getMilestoneIDs() []int64 {
	ids := make(map[int64]struct{}, len(comments))
	for _, comment := range comments {
		if _, ok := ids[comment.MilestoneID]; !ok {
			ids[comment.MilestoneID] = struct{}{}
		}
	}
	return keysInt64(ids)
}

func (comments CommentList) loadMilestones(e db.Engine) error {
	if len(comments) == 0 {
		return nil
	}

	milestoneIDs := comments.getMilestoneIDs()
	if len(milestoneIDs) == 0 {
		return nil
	}

	milestoneMaps := make(map[int64]*Milestone, len(milestoneIDs))
	left := len(milestoneIDs)
	for left > 0 {
		limit := defaultMaxInSize
		if left < limit {
			limit = left
		}
		err := e.
			In("id", milestoneIDs[:limit]).
			Find(&milestoneMaps)
		if err != nil {
			return err
		}
		left -= limit
		milestoneIDs = milestoneIDs[limit:]
	}

	for _, issue := range comments {
		issue.Milestone = milestoneMaps[issue.MilestoneID]
	}
	return nil
}

func (comments CommentList) getOldMilestoneIDs() []int64 {
	ids := make(map[int64]struct{}, len(comments))
	for _, comment := range comments {
		if _, ok := ids[comment.OldMilestoneID]; !ok {
			ids[comment.OldMilestoneID] = struct{}{}
		}
	}
	return keysInt64(ids)
}

func (comments CommentList) loadOldMilestones(e db.Engine) error {
	if len(comments) == 0 {
		return nil
	}

	milestoneIDs := comments.getOldMilestoneIDs()
	if len(milestoneIDs) == 0 {
		return nil
	}

	milestoneMaps := make(map[int64]*Milestone, len(milestoneIDs))
	left := len(milestoneIDs)
	for left > 0 {
		limit := defaultMaxInSize
		if left < limit {
			limit = left
		}
		err := e.
			In("id", milestoneIDs[:limit]).
			Find(&milestoneMaps)
		if err != nil {
			return err
		}
		left -= limit
		milestoneIDs = milestoneIDs[limit:]
	}

	for _, issue := range comments {
		issue.OldMilestone = milestoneMaps[issue.MilestoneID]
	}
	return nil
}

func (comments CommentList) getAssigneeIDs() []int64 {
	ids := make(map[int64]struct{}, len(comments))
	for _, comment := range comments {
		if _, ok := ids[comment.AssigneeID]; !ok {
			ids[comment.AssigneeID] = struct{}{}
		}
	}
	return keysInt64(ids)
}

func (comments CommentList) loadAssignees(e db.Engine) error {
	if len(comments) == 0 {
		return nil
	}

	assigneeIDs := comments.getAssigneeIDs()
	assignees := make(map[int64]*User, len(assigneeIDs))
	left := len(assigneeIDs)
	for left > 0 {
		limit := defaultMaxInSize
		if left < limit {
			limit = left
		}
		rows, err := e.
			In("id", assigneeIDs[:limit]).
			Rows(new(User))
		if err != nil {
			return err
		}

		for rows.Next() {
			var user User
			err = rows.Scan(&user)
			if err != nil {
				rows.Close()
				return err
			}

			assignees[user.ID] = &user
		}
		_ = rows.Close()

		left -= limit
		assigneeIDs = assigneeIDs[limit:]
	}

	for _, comment := range comments {
		comment.Assignee = assignees[comment.AssigneeID]
	}
	return nil
}

// getIssueIDs returns all the issue ids on this comment list which issue hasn't been loaded
func (comments CommentList) getIssueIDs() []int64 {
	ids := make(map[int64]struct{}, len(comments))
	for _, comment := range comments {
		if comment.Issue != nil {
			continue
		}
		if _, ok := ids[comment.IssueID]; !ok {
			ids[comment.IssueID] = struct{}{}
		}
	}
	return keysInt64(ids)
}

// Issues returns all the issues of comments
func (comments CommentList) Issues() IssueList {
	issues := make(map[int64]*Issue, len(comments))
	for _, comment := range comments {
		if comment.Issue != nil {
			if _, ok := issues[comment.Issue.ID]; !ok {
				issues[comment.Issue.ID] = comment.Issue
			}
		}
	}

	issueList := make([]*Issue, 0, len(issues))
	for _, issue := range issues {
		issueList = append(issueList, issue)
	}
	return issueList
}

func (comments CommentList) loadIssues(e db.Engine) error {
	if len(comments) == 0 {
		return nil
	}

	issueIDs := comments.getIssueIDs()
	issues := make(map[int64]*Issue, len(issueIDs))
	left := len(issueIDs)
	for left > 0 {
		limit := defaultMaxInSize
		if left < limit {
			limit = left
		}
		rows, err := e.
			In("id", issueIDs[:limit]).
			Rows(new(Issue))
		if err != nil {
			return err
		}

		for rows.Next() {
			var issue Issue
			err = rows.Scan(&issue)
			if err != nil {
				rows.Close()
				return err
			}

			issues[issue.ID] = &issue
		}
		_ = rows.Close()

		left -= limit
		issueIDs = issueIDs[limit:]
	}

	for _, comment := range comments {
		if comment.Issue == nil {
			comment.Issue = issues[comment.IssueID]
		}
	}
	return nil
}

func (comments CommentList) getDependentIssueIDs() []int64 {
	ids := make(map[int64]struct{}, len(comments))
	for _, comment := range comments {
		if comment.DependentIssue != nil {
			continue
		}
		if _, ok := ids[comment.DependentIssueID]; !ok {
			ids[comment.DependentIssueID] = struct{}{}
		}
	}
	return keysInt64(ids)
}

func (comments CommentList) loadDependentIssues(e db.Engine) error {
	if len(comments) == 0 {
		return nil
	}

	issueIDs := comments.getDependentIssueIDs()
	issues := make(map[int64]*Issue, len(issueIDs))
	left := len(issueIDs)
	for left > 0 {
		limit := defaultMaxInSize
		if left < limit {
			limit = left
		}
		rows, err := e.
			In("id", issueIDs[:limit]).
			Rows(new(Issue))
		if err != nil {
			return err
		}

		for rows.Next() {
			var issue Issue
			err = rows.Scan(&issue)
			if err != nil {
				_ = rows.Close()
				return err
			}

			issues[issue.ID] = &issue
		}
		_ = rows.Close()

		left -= limit
		issueIDs = issueIDs[limit:]
	}

	for _, comment := range comments {
		if comment.DependentIssue == nil {
			comment.DependentIssue = issues[comment.DependentIssueID]
			if comment.DependentIssue != nil {
				if err := comment.DependentIssue.loadRepo(e); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (comments CommentList) loadAttachments(e db.Engine) (err error) {
	if len(comments) == 0 {
		return nil
	}

	attachments := make(map[int64][]*Attachment, len(comments))
	commentsIDs := comments.getCommentIDs()
	left := len(commentsIDs)
	for left > 0 {
		limit := defaultMaxInSize
		if left < limit {
			limit = left
		}
		rows, err := e.Table("attachment").
			Join("INNER", "comment", "comment.id = attachment.comment_id").
			In("comment.id", commentsIDs[:limit]).
			Rows(new(Attachment))
		if err != nil {
			return err
		}

		for rows.Next() {
			var attachment Attachment
			err = rows.Scan(&attachment)
			if err != nil {
				_ = rows.Close()
				return err
			}
			attachments[attachment.CommentID] = append(attachments[attachment.CommentID], &attachment)
		}

		_ = rows.Close()
		left -= limit
		commentsIDs = commentsIDs[limit:]
	}

	for _, comment := range comments {
		comment.Attachments = attachments[comment.ID]
	}
	return nil
}

func (comments CommentList) getReviewIDs() []int64 {
	ids := make(map[int64]struct{}, len(comments))
	for _, comment := range comments {
		if _, ok := ids[comment.ReviewID]; !ok {
			ids[comment.ReviewID] = struct{}{}
		}
	}
	return keysInt64(ids)
}

func (comments CommentList) loadReviews(e db.Engine) error {
	if len(comments) == 0 {
		return nil
	}

	reviewIDs := comments.getReviewIDs()
	reviews := make(map[int64]*Review, len(reviewIDs))
	left := len(reviewIDs)
	for left > 0 {
		limit := defaultMaxInSize
		if left < limit {
			limit = left
		}
		rows, err := e.
			In("id", reviewIDs[:limit]).
			Rows(new(Review))
		if err != nil {
			return err
		}

		for rows.Next() {
			var review Review
			err = rows.Scan(&review)
			if err != nil {
				_ = rows.Close()
				return err
			}

			reviews[review.ID] = &review
		}
		_ = rows.Close()

		left -= limit
		reviewIDs = reviewIDs[limit:]
	}

	for _, comment := range comments {
		comment.Review = reviews[comment.ReviewID]
	}
	return nil
}

// loadAttributes loads all attributes
func (comments CommentList) loadAttributes(e db.Engine) (err error) {
	if err = comments.loadPosters(e); err != nil {
		return
	}

	if err = comments.loadLabels(e); err != nil {
		return
	}

	if err = comments.loadMilestones(e); err != nil {
		return
	}

	if err = comments.loadOldMilestones(e); err != nil {
		return
	}

	if err = comments.loadAssignees(e); err != nil {
		return
	}

	if err = comments.loadAttachments(e); err != nil {
		return
	}

	if err = comments.loadReviews(e); err != nil {
		return
	}

	if err = comments.loadIssues(e); err != nil {
		return
	}

	if err = comments.loadDependentIssues(e); err != nil {
		return
	}

	return nil
}

// LoadAttributes loads attributes of the comments, except for attachments and
// comments
func (comments CommentList) LoadAttributes() error {
	return comments.loadAttributes(db.DefaultContext().Engine())
}

// LoadAttachments loads attachments
func (comments CommentList) LoadAttachments() error {
	return comments.loadAttachments(db.DefaultContext().Engine())
}

// LoadPosters loads posters
func (comments CommentList) LoadPosters() error {
	return comments.loadPosters(db.DefaultContext().Engine())
}

// LoadIssues loads issues of comments
func (comments CommentList) LoadIssues() error {
	return comments.loadIssues(db.DefaultContext().Engine())
}
