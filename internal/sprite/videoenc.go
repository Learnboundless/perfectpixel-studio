package sprite

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
)

// EncodeMP4 encodes processed transparent frames over a keyable magenta
// background. H.264 MP4 has no portable alpha channel; GIF/APNG remain the
// alpha-preserving export choices.
func EncodeMP4(ctx context.Context, frames []*image.NRGBA, fps int) ([]byte, error) {
	if len(frames) == 0 {
		return nil, fmt.Errorf("没有可导出的视频帧")
	}
	if fps < 1 {
		fps = 8
	}
	ffmpeg, err := ffmpegPath()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "perfectpixel-export-video-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	for i, frame := range frames {
		var buf bytes.Buffer
		if err := png.Encode(&buf, frame); err != nil {
			return nil, err
		}
		path := filepath.Join(dir, fmt.Sprintf("frame-%06d.png", i))
		if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
			return nil, err
		}
	}
	w, h := frames[0].Rect.Dx(), frames[0].Rect.Dy()
	output := filepath.Join(dir, "animation.mp4")
	pattern := filepath.Join(dir, "frame-%06d.png")
	filter := fmt.Sprintf("color=c=0xFF00FF:s=%dx%d:r=%d[bg];[bg][0:v]overlay=shortest=1:format=auto,format=yuv420p", w, h, fps)
	args := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-framerate", fmt.Sprintf("%d", fps), "-i", pattern,
		"-filter_complex", filter,
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart",
		output,
	}
	if out, err := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("MP4 编码失败: %w%s", err, ffmpegOutputSuffix(out))
	}
	return os.ReadFile(output)
}
