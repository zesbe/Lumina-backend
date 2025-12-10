package services

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrMiniMaxAPIKeyMissing = errors.New("MiniMax API key is not configured")
	ErrMiniMaxRequestFailed = errors.New("MiniMax API request failed")
	ErrMiniMaxJobFailed     = errors.New("MiniMax job failed")
	ErrNarrationTooLong     = errors.New("narration too long for video duration")
)

type MiniMaxService struct {
	apiKey     string
	groupID    string
	httpClient *http.Client
	baseURL    string
}

type AudioSetting struct {
	Channel    int    `json:"channel"`
	SampleRate int    `json:"sample_rate"`
	Bitrate    int    `json:"bitrate"`
	Format     string `json:"format"`
}

type MusicGenerationRequest struct {
	Model        string       `json:"model"`
	Prompt       string       `json:"prompt"`
	Lyrics       string       `json:"lyrics,omitempty"`
	AudioSetting AudioSetting `json:"audio_setting"`
}

type VideoGenerationRequest struct {
	Model      string `json:"model"`
	Prompt     string `json:"prompt"`
	Duration   int    `json:"duration,omitempty"`
	Resolution string `json:"resolution,omitempty"`
}

type TTSRequest struct {
	Model        string          `json:"model"`
	Text         string          `json:"text"`
	VoiceSetting TTSVoiceSetting `json:"voice_setting"`
	AudioSetting TTSAudioSetting `json:"audio_setting"`
}

type TTSVoiceSetting struct {
	VoiceID string  `json:"voice_id"`
	Speed   float64 `json:"speed"`
	Vol     float64 `json:"vol"`
	Pitch   int     `json:"pitch"`
}

type TTSAudioSetting struct {
	SampleRate int    `json:"sample_rate"`
	Bitrate    int    `json:"bitrate"`
	Format     string `json:"format"`
}

type ImageGenerationRequest struct {
	Model       string `json:"model"`
	Prompt      string `json:"prompt"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
}

type ImageGenerationResponse struct {
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
	Data struct {
		ImageURLs []string `json:"image_urls"`
	} `json:"data"`
}

type MusicResponse struct {
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
	Data struct {
		Audio string `json:"audio"`
	} `json:"data"`
	ExtraInfo json.RawMessage `json:"extra_info"`
}

type VideoResponse struct {
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
	TaskID string `json:"task_id"`
}

type TTSResponse struct {
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
	Data struct {
		Audio string `json:"audio"`
	} `json:"data"`
	ExtraInfo struct {
		AudioLength int `json:"audio_length"`
		AudioSize   int `json:"audio_size"`
	} `json:"extra_info"`
}

type MiniMaxTaskStatus struct {
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
	Status string `json:"status"`
	FileID string `json:"file_id"`
	File   struct {
		FileID      int64  `json:"file_id"`
		DownloadURL string `json:"download_url"`
	} `json:"file"`
	ExtraInfo json.RawMessage `json:"extra_info"`
}

type FileRetrieveResponse struct {
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
	File struct {
		FileID      int64  `json:"file_id"`
		Bytes       int64  `json:"bytes"`
		CreatedAt   int64  `json:"created_at"`
		Filename    string `json:"filename"`
		Purpose     string `json:"purpose"`
		DownloadURL string `json:"download_url"`
	} `json:"file"`
}

func NewMiniMaxService(apiKey, groupID string) *MiniMaxService {
	return &MiniMaxService{
		apiKey:  apiKey,
		groupID: groupID,
		httpClient: &http.Client{
			Timeout: 480 * time.Second,
		},
		baseURL: "https://api.minimaxi.chat/v1",
	}
}

func (s *MiniMaxService) IsConfigured() bool {
	return s.apiKey != ""
}

func EstimateTTSDuration(text string) float64 {
	words := len(strings.Fields(text))
	return float64(words) / 2.5
}

func CalculateOptimalSpeed(text string, videoDuration int) (float64, error) {
	estimatedDuration := EstimateTTSDuration(text)
	targetDuration := float64(videoDuration) - 0.5

	if targetDuration <= 0 {
		targetDuration = float64(videoDuration)
	}

	if estimatedDuration <= targetDuration {
		return 1.0, nil
	}

	requiredSpeed := estimatedDuration / targetDuration

	if requiredSpeed > 1.3 {
		if requiredSpeed > 1.5 {
			return 0, ErrNarrationTooLong
		}
		return 1.3, nil
	}

	return float64(int(requiredSpeed*10)) / 10, nil
}

func (s *MiniMaxService) GenerateMusic(prompt, lyrics, format, model string, bitrate int) (*MusicResponse, error) {
	if !s.IsConfigured() {
		return nil, ErrMiniMaxAPIKeyMissing
	}

	reqBody := MusicGenerationRequest{
		Model:  model,
		Prompt: prompt,
		Lyrics: lyrics,
		AudioSetting: AudioSetting{
			SampleRate: 44100,
			Channel:    2,
			Bitrate:    bitrate,
			Format:     format,
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := "https://api.minimax.io/v1/music_generation"

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result MusicResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	if result.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("%w: %s", ErrMiniMaxRequestFailed, result.BaseResp.StatusMsg)
	}

	return &result, nil
}

func (s *MiniMaxService) GenerateImage(prompt string) (string, error) {
	if !s.IsConfigured() {
		return "", ErrMiniMaxAPIKeyMissing
	}

	reqBody := ImageGenerationRequest{
		Model:       "image-01",
		Prompt:      prompt,
		AspectRatio: "1:1",
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/image_generation?GroupId=%s", s.baseURL, s.groupID)
	log.Printf("[MiniMax] Image generation started")

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	log.Printf("[MiniMax] Image response: %s", string(body)[:200])

	var result ImageGenerationResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse image response: %v", err)
	}

	if result.BaseResp.StatusCode != 0 {
		return "", fmt.Errorf("%w: %s", ErrMiniMaxRequestFailed, result.BaseResp.StatusMsg)
	}

	if len(result.Data.ImageURLs) > 0 {
		return result.Data.ImageURLs[0], nil
	}

	return "", fmt.Errorf("no image generated")
}

func (s *MiniMaxService) GenerateTTS(text string, voiceID string) (*TTSResponse, error) {
	return s.GenerateTTSWithSpeed(text, voiceID, 1.0)
}

func (s *MiniMaxService) GenerateTTSWithSpeed(text string, voiceID string, speed float64) (*TTSResponse, error) {
	if !s.IsConfigured() {
		return nil, ErrMiniMaxAPIKeyMissing
	}

	if voiceID == "" {
		voiceID = "male-qn-qingse"
	}

	if speed < 0.5 {
		speed = 0.5
	}
	if speed > 2.0 {
		speed = 2.0
	}

	reqBody := TTSRequest{
		Model: "speech-01-turbo",
		Text:  text,
		VoiceSetting: TTSVoiceSetting{
			VoiceID: voiceID,
			Speed:   speed,
			Vol:     1.0,
			Pitch:   0,
		},
		AudioSetting: TTSAudioSetting{
			SampleRate: 32000,
			Bitrate:    128000,
			Format:     "mp3",
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://api.minimax.io/v1/t2a_v2?GroupId=%s", s.groupID)
	log.Printf("[TTS] Generating with speed: %.1fx, text length: %d chars", speed, len(text))

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result TTSResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse TTS response: %v", err)
	}

	if result.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("%w: %s", ErrMiniMaxRequestFailed, result.BaseResp.StatusMsg)
	}

	return &result, nil
}

func (s *MiniMaxService) GenerateVideo(prompt string, duration int, resolution string, model string) (*VideoResponse, error) {
	if !s.IsConfigured() {
		return nil, ErrMiniMaxAPIKeyMissing
	}

	if model == "" {
		model = "video-01"
	}

	maxDuration := 6
	if (model == "MiniMax-Hailuo-02" || model == "hailuo-02") && resolution != "1080P" {
		maxDuration = 10
	}

	if duration <= 0 {
		duration = 6
	}
	if duration > maxDuration {
		duration = maxDuration
	}

	if duration > 6 {
		resolution = "768P"
	}

	reqBody := VideoGenerationRequest{
		Model:    model,
		Prompt:   prompt,
		Duration: duration,
	}

	if model == "MiniMax-Hailuo-02" || model == "hailuo-02" {
		if resolution == "" {
			resolution = "768P"
		}
		reqBody.Resolution = resolution
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/video_generation?GroupId=%s", s.baseURL, s.groupID)
	log.Printf("[MiniMax] Video - Model: %s, Duration: %d, Resolution: %s", model, duration, resolution)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result VideoResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse: %v", err)
	}

	if result.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("%w: %s", ErrMiniMaxRequestFailed, result.BaseResp.StatusMsg)
	}

	return &result, nil
}

func (s *MiniMaxService) GetTaskStatus(taskID string) (*MiniMaxTaskStatus, error) {
	if !s.IsConfigured() {
		return nil, ErrMiniMaxAPIKeyMissing
	}

	url := fmt.Sprintf("%s/query/video_generation?task_id=%s", s.baseURL, taskID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result MiniMaxTaskStatus
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if result.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("%w: %s", ErrMiniMaxRequestFailed, result.BaseResp.StatusMsg)
	}

	return &result, nil
}

func (s *MiniMaxService) GetFileDownloadURL(fileID string) (string, error) {
	if !s.IsConfigured() {
		return "", ErrMiniMaxAPIKeyMissing
	}

	url := fmt.Sprintf("%s/files/retrieve?file_id=%s", s.baseURL, fileID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result FileRetrieveResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	if result.BaseResp.StatusCode != 0 {
		return "", fmt.Errorf("%w: %s", ErrMiniMaxRequestFailed, result.BaseResp.StatusMsg)
	}

	return result.File.DownloadURL, nil
}

func (s *MiniMaxService) WaitForCompletion(taskID string, timeout time.Duration) (*MiniMaxTaskStatus, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if time.Now().After(deadline) {
				return nil, errors.New("timeout")
			}

			status, err := s.GetTaskStatus(taskID)
			if err != nil {
				continue
			}

			log.Printf("[MiniMax] Task %s: %s", taskID, status.Status)

			switch status.Status {
			case "Success", "Completed":
				if status.FileID != "" {
					url, err := s.GetFileDownloadURL(status.FileID)
					if err != nil {
						return nil, err
					}
					status.File.DownloadURL = url
				}
				return status, nil
			case "Failed", "Error":
				return nil, ErrMiniMaxJobFailed
			}
		}
	}
}

func (s *MiniMaxService) CombineVideoWithAudio(videoURL string, audioHex string, outputPath string) error {
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("lumina_%d", time.Now().UnixNano()))
	os.MkdirAll(tempDir, 0755)
	defer os.RemoveAll(tempDir)

	videoPath := filepath.Join(tempDir, "video.mp4")
	if err := downloadFile(videoURL, videoPath); err != nil {
		return err
	}

	audioPath := filepath.Join(tempDir, "audio.mp3")
	audioBytes, _ := hex.DecodeString(audioHex)
	os.WriteFile(audioPath, audioBytes, 0644)

	cmd := exec.Command("ffmpeg", "-y", "-i", videoPath, "-i", audioPath, "-c:v", "copy", "-c:a", "aac", "-shortest", outputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg: %s", string(output))
	}

	return nil
}

func downloadFile(url string, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, _ := os.Create(filepath)
	defer out.Close()

	io.Copy(out, resp.Body)
	return nil
}
