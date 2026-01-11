package drive

import (
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/driveactivity/v2"
)

// ChangeInfo represents simplified change information.
type ChangeInfo struct {
	FileID     string
	FileName   string
	ChangeTime time.Time
	ChangeType string
	Removed    bool
	MimeType   string
	ModifiedBy string
}

// ListChanges lists recent changes to files in the Drive.
func (ds *Service) ListChanges(pageSize int64) ([]*ChangeInfo, error) {
	// Get the start page token
	startToken, err := ds.API.Changes.GetStartPageToken().Do()
	if err != nil {
		return nil, fmt.Errorf("unable to get start page token: %v", err)
	}

	// List changes from the start token
	changeList, err := ds.API.Changes.List(startToken.StartPageToken).
		PageSize(pageSize).
		Fields("changes(file(id, name, mimeType, modifiedTime, modifiedByMeTime, lastModifyingUser), fileId, removed, time)").
		Do()
	if err != nil {
		return nil, fmt.Errorf("unable to list changes: %v", err)
	}

	var changes []*ChangeInfo
	for _, change := range changeList.Changes {
		changeInfo := &ChangeInfo{
			FileID:     change.FileId,
			Removed:    change.Removed,
			ChangeTime: time.Now(), // Default to now if not available
		}

		if change.Time != "" {
			if t, err := time.Parse(time.RFC3339, change.Time); err == nil {
				changeInfo.ChangeTime = t
			}
		}

		if change.File != nil {
			changeInfo.FileName = change.File.Name
			changeInfo.MimeType = change.File.MimeType

			if change.File.LastModifyingUser != nil {
				changeInfo.ModifiedBy = change.File.LastModifyingUser.DisplayName
				if changeInfo.ModifiedBy == "" {
					changeInfo.ModifiedBy = change.File.LastModifyingUser.EmailAddress
				}
			}

			// Determine change type
			if change.Removed {
				changeInfo.ChangeType = "Removed"
			} else if change.File.ModifiedTime != "" {
				changeInfo.ChangeType = "Modified"
			} else {
				changeInfo.ChangeType = "Added"
			}
		}

		changes = append(changes, changeInfo)
	}

	return changes, nil
}

// ListTrashedFiles lists files in the trash, optionally filtered by time.
func (ds *Service) ListTrashedFiles(daysBack int, maxResults int64) ([]*drive.File, error) {
	// Build query for trashed files
	query := "trashed = true"

	// If days back is specified, add time filter
	if daysBack > 0 {
		cutoffTime := time.Now().AddDate(0, 0, -daysBack).Format(time.RFC3339)
		query = fmt.Sprintf("trashed = true and trashedTime >= '%s'", cutoffTime)
	}

	fileList, err := ds.API.Files.List().
		Q(query).
		PageSize(maxResults).
		Fields("files(id, name, mimeType, trashedTime, trashingUser, size, parents)").
		OrderBy("trashedTime desc").
		Do()

	if err != nil {
		return nil, fmt.Errorf("unable to list trashed files: %v", err)
	}

	return fileList.Files, nil
}

// RevisionInfo represents file revision information.
type RevisionInfo struct {
	ID           string
	ModifiedTime time.Time
	Size         int64
	MimeType     string
	ModifiedBy   string
	KeepForever  bool
	Published    bool
}

// ListRevisions lists all revisions for a specific file.
func (ds *Service) ListRevisions(fileID string) ([]*RevisionInfo, error) {
	revList, err := ds.API.Revisions.List(fileID).
		Fields("revisions(id, modifiedTime, size, mimeType, lastModifyingUser, keepForever, published)").
		Do()
	if err != nil {
		return nil, fmt.Errorf("unable to list revisions: %v", err)
	}

	var revisions []*RevisionInfo
	for _, rev := range revList.Revisions {
		revInfo := &RevisionInfo{
			ID:          rev.Id,
			Size:        rev.Size,
			MimeType:    rev.MimeType,
			KeepForever: rev.KeepForever,
			Published:   rev.Published,
		}

		if rev.ModifiedTime != "" {
			if t, err := time.Parse(time.RFC3339, rev.ModifiedTime); err == nil {
				revInfo.ModifiedTime = t
			}
		}

		if rev.LastModifyingUser != nil {
			revInfo.ModifiedBy = rev.LastModifyingUser.DisplayName
			if revInfo.ModifiedBy == "" {
				revInfo.ModifiedBy = rev.LastModifyingUser.EmailAddress
			}
		}

		revisions = append(revisions, revInfo)
	}

	return revisions, nil
}

// GetRevision gets a specific revision of a file.
func (ds *Service) GetRevision(fileID, revisionID string) (*RevisionInfo, error) {
	rev, err := ds.API.Revisions.Get(fileID, revisionID).
		Fields("id, modifiedTime, size, mimeType, lastModifyingUser, keepForever, published").
		Do()
	if err != nil {
		return nil, fmt.Errorf("unable to get revision: %v", err)
	}

	revInfo := &RevisionInfo{
		ID:          rev.Id,
		Size:        rev.Size,
		MimeType:    rev.MimeType,
		KeepForever: rev.KeepForever,
		Published:   rev.Published,
	}

	if rev.ModifiedTime != "" {
		if t, err := time.Parse(time.RFC3339, rev.ModifiedTime); err == nil {
			revInfo.ModifiedTime = t
		}
	}

	if rev.LastModifyingUser != nil {
		revInfo.ModifiedBy = rev.LastModifyingUser.DisplayName
		if revInfo.ModifiedBy == "" {
			revInfo.ModifiedBy = rev.LastModifyingUser.EmailAddress
		}
	}

	return revInfo, nil
}

// DriveActivityInfo represents a drive activity event.
type DriveActivityInfo struct {
	Timestamp    time.Time
	ActionType   string
	ActionDetail string
	Actors       []string
	Targets      []string
	TargetTitles []string
}

// QueryDriveActivity queries the Drive Activity API for recent activities.
func QueryDriveActivity(activityService *driveactivity.Service, daysBack int, maxResults int64) ([]*DriveActivityInfo, error) {
	// Build the request
	pageSize := int64(100) // API max per page
	if maxResults > 0 && maxResults < pageSize {
		pageSize = maxResults
	}

	req := &driveactivity.QueryDriveActivityRequest{
		PageSize: pageSize,
	}

	// Add time filter if specified
	if daysBack > 0 {
		cutoffTime := time.Now().AddDate(0, 0, -daysBack)
		req.Filter = fmt.Sprintf("time >= \"%s\"", cutoffTime.Format(time.RFC3339))
	}

	var activities []*DriveActivityInfo
	pageToken := ""
	pageCount := 0
	maxPagesPerBatch := 90 // Stay under 100 queries/minute limit

	// Fetch all pages until we reach maxResults or no more pages
	for {
		if pageToken != "" {
			req.PageToken = pageToken
		}

		// Query activities with retry logic for rate limiting
		var resp *driveactivity.QueryDriveActivityResponse
		var err error
		maxRetries := 3
		baseDelay := 2 * time.Second

		for retry := 0; retry <= maxRetries; retry++ {
			resp, err = activityService.Activity.Query(req).Do()
			if err == nil {
				break
			}

			// Check if it's a rate limit error
			if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "rateLimitExceeded") {
				if retry < maxRetries {
					// Exponential backoff: 2s, 4s, 8s
					delay := baseDelay * time.Duration(1<<uint(retry))
					fmt.Fprintf(os.Stderr, "⚠ Rate limit hit. Waiting %v before retry %d/%d...\n", delay, retry+1, maxRetries)
					time.Sleep(delay)
					continue
				}
			}
			return nil, fmt.Errorf("unable to query drive activity: %v", err)
		}

		pageCount++

		// Rate limiting: if we've made many requests, pause
		if pageCount%maxPagesPerBatch == 0 {
			fmt.Fprintf(os.Stderr, "⏸  Fetched %d pages (%d activities). Pausing 60s to respect rate limits...\n",
				pageCount, len(activities))
			time.Sleep(60 * time.Second)
		}

		// Process activities from this page
		for _, activity := range resp.Activities {
			activityInfo := &DriveActivityInfo{
				Actors:       []string{},
				Targets:      []string{},
				TargetTitles: []string{},
			}

			// Parse timestamp
			if activity.Timestamp != "" {
				if t, err := time.Parse(time.RFC3339Nano, activity.Timestamp); err == nil {
					activityInfo.Timestamp = t
				}
			}

			// Parse actors
			if activity.Actors != nil {
				for _, actor := range activity.Actors {
					if actor.User != nil && actor.User.KnownUser != nil {
						if actor.User.KnownUser.PersonName != "" {
							activityInfo.Actors = append(activityInfo.Actors, actor.User.KnownUser.PersonName)
						}
					} else if actor.Administrator != nil {
						activityInfo.Actors = append(activityInfo.Actors, "Administrator")
					} else if actor.Anonymous != nil {
						activityInfo.Actors = append(activityInfo.Actors, "Anonymous")
					}
				}
			}

			// Parse targets
			if activity.Targets != nil {
				for _, target := range activity.Targets {
					if target.DriveItem != nil {
						// Try Title first (this is the actual file name)
						if target.DriveItem.Title != "" {
							activityInfo.TargetTitles = append(activityInfo.TargetTitles, target.DriveItem.Title)
						} else if target.DriveItem.Name != "" {
							// Name is the resource ID, use only if Title is not available
							// Extract just the ID part for display
							name := target.DriveItem.Name
							if len(name) > 6 && name[:6] == "items/" {
								name = name[6:] // Remove "items/" prefix
							}
							activityInfo.TargetTitles = append(activityInfo.TargetTitles, fmt.Sprintf("<ID: %s>", name))
						}
						// Store the resource name/ID for reference
						if target.DriveItem.Name != "" {
							activityInfo.Targets = append(activityInfo.Targets, target.DriveItem.Name)
						}
					}
					if target.FileComment != nil && target.FileComment.Parent != nil {
						activityInfo.Targets = append(activityInfo.Targets, "Comment")
					}
				}
			}

			// Parse primary action
			if activity.PrimaryActionDetail != nil {
				activityInfo.ActionType, activityInfo.ActionDetail = parsePrimaryAction(activity.PrimaryActionDetail)
			}

			activities = append(activities, activityInfo)

			// Check if we've reached maxResults
			if maxResults > 0 && int64(len(activities)) >= maxResults {
				return activities, nil
			}
		}

		// Check if there are more pages
		pageToken = resp.NextPageToken
		if pageToken == "" {
			// No more pages
			break
		}

		// Check if we've reached maxResults
		if maxResults > 0 && int64(len(activities)) >= maxResults {
			break
		}
	}

	return activities, nil
}

// parsePrimaryAction extracts action type and details from PrimaryActionDetail.
func parsePrimaryAction(action *driveactivity.ActionDetail) (string, string) {
	if action.Create != nil {
		if action.Create.New != nil {
			return "Create", "Created new item"
		}
		if action.Create.Upload != nil {
			return "Upload", "Uploaded file"
		}
		if action.Create.Copy != nil {
			return "Copy", "Copied file"
		}
	}

	if action.Edit != nil {
		return "Edit", "Edited file"
	}

	if action.Move != nil {
		detail := "Moved"
		if action.Move.AddedParents != nil && len(action.Move.AddedParents) > 0 {
			detail += " to new location"
		}
		if action.Move.RemovedParents != nil && len(action.Move.RemovedParents) > 0 {
			detail += " from old location"
		}
		return "Move", detail
	}

	if action.Rename != nil {
		detail := "Renamed"
		if action.Rename.OldTitle != "" && action.Rename.NewTitle != "" {
			detail = fmt.Sprintf("Renamed from '%s' to '%s'", action.Rename.OldTitle, action.Rename.NewTitle)
		}
		return "Rename", detail
	}

	if action.Delete != nil {
		deleteType := "Deleted"
		if action.Delete.Type == "TRASH" {
			deleteType = "Moved to trash"
		} else if action.Delete.Type == "PERMANENT_DELETE" {
			deleteType = "Permanently deleted"
		}
		return "Delete", deleteType
	}

	if action.Restore != nil {
		return "Restore", "Restored from trash"
	}

	if action.PermissionChange != nil {
		detail := "Changed permissions"
		if action.PermissionChange.AddedPermissions != nil && len(action.PermissionChange.AddedPermissions) > 0 {
			detail = "Added permissions"
		}
		if action.PermissionChange.RemovedPermissions != nil && len(action.PermissionChange.RemovedPermissions) > 0 {
			detail = "Removed permissions"
		}
		return "Permission", detail
	}

	if action.Comment != nil {
		if action.Comment.Post != nil {
			return "Comment", "Posted comment"
		}
		if action.Comment.Assignment != nil {
			return "Comment", "Assigned task"
		}
	}

	if action.DlpChange != nil {
		return "DLP", "Data loss prevention change"
	}

	if action.Reference != nil {
		return "Reference", "Referenced in another document"
	}

	if action.SettingsChange != nil {
		return "Settings", "Changed settings"
	}

	return "Unknown", "Unknown action"
}
