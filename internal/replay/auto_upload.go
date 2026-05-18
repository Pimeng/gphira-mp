package replay

import (
	"os"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/config"
	"github.com/Pimeng/gphira-mp-next/internal/utils"
)

// AutoUploadMeta holds metadata for an uploaded replay.
type AutoUploadMeta struct {
	ScoreID   int32
	ChartID   int32
	Timestamp int64
}

// AutoUploadCallback is called when a game ends to potentially auto-upload replays.
type AutoUploadCallback func(userID int32, chartID int32, timestamp int64, recordID int32)

// CreateAutoUploadHandler creates a handler that uploads replays after a delay.
func CreateAutoUploadHandler(
	cfg *config.ServerConfig,
	logger *utils.Logger,
	uploadedMeta *utils.UploadedReplayMeta,
	uploadConfigs *utils.AutoUploadConfigs,
) AutoUploadCallback {
	return func(userID int32, chartID int32, timestamp int64, recordID int32) {
		if !config.DerefBool(cfg.ReplayAutoUpload, false) {
			if logger != nil {
				logger.Debug("Auto upload skipped: REPLAY_AUTO_UPLOAD disabled")
			}
			return
		}
		if cfg.ShareStation == nil || cfg.ShareStation.URL == "" || cfg.ShareStation.Token == "" {
			if logger != nil {
				logger.Debug("Auto upload skipped: share station not configured")
			}
			return
		}

		// Delay 30s before uploading
		time.AfterFunc(30*time.Second, func() {
			if !config.DerefBool(cfg.ReplayAutoUpload, false) {
				return
			}

			filePath := ReplayFilePath(cfg.ReplayBaseDir, userID, chartID, timestamp)
			header, err := ReadReplayHeader(filePath)
			if err != nil || header.UserID != userID || header.ChartID != chartID {
				if logger != nil {
					logger.Warn("Auto upload failed: replay file not found or invalid", "user", userID)
				}
				return
			}

			fileData, err := os.ReadFile(filePath)
			if err != nil {
				if logger != nil {
					logger.Warn("Auto upload failed: failed to read file", "user", userID, "err", err)
				}
				return
			}

			result, err := utils.UploadToShareStation(
				fileData,
				formatFilename(timestamp),
				header.ChartName,
				header.UserName,
				cfg.ShareStation,
				cfg.OutboundProxy,
			)
			if err != nil || !result.Success {
				msg := "upload-failed"
				if result != nil {
					msg = result.Message
				}
				if logger != nil {
					logger.Warn("Auto upload failed", "user", userID, "reason", msg)
				}
				return
			}

			// Store metadata
			if result.ScoreID != 0 && uploadedMeta != nil {
				uploadedMeta.Add(userID, chartID, result.ScoreID, timestamp)
			}

			// Set visibility based on user config
			if result.ScoreID != 0 && uploadConfigs != nil {
				if uploadConfigs.GetShow(userID) {
					utils.SetReplayVisibility(result.ScoreID, true, cfg.ShareStation, cfg.OutboundProxy)
				}
			}

			if logger != nil {
				logger.Info("Auto upload completed", "user", userID, "chart", chartID, "scoreId", result.ScoreID)
			}

			// Delete local file after successful upload
			if err := os.Remove(filePath); err != nil {
				if logger != nil {
					logger.Warn("Failed to delete local replay file after upload", "user", userID, "err", err)
				}
			}
		})
	}
}

func formatFilename(timestamp int64) string {
	return i64toa(timestamp) + ".phirarec"
}
