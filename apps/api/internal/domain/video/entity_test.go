package domainvideo

import (
	"errors"
	"strings"
	"testing"
)

// TestVideoCharacterLimits 校验中英文和非 BMP 字符使用一致的字数上限。
func TestVideoCharacterLimits(t *testing.T) {
	for _, unit := range []string{"中", "x", "🎬"} {
		for _, field := range []struct {
			name string
			max  int
			err  error
		}{
			{"title", MaxTitleLength, ErrTitleTooLong},
			{"description", MaxDescriptionLength, ErrDescriptionTooLong},
			{"model", MaxModelNameLength, ErrModelNameTooLong},
			{"params", MaxModelParamsLength, ErrModelParamsTooLong},
			{"style", MaxAIStyleTagLength, ErrAIStyleTagTooLong},
		} {
			for _, extra := range []int{0, 1} {
				t.Run(field.name+"/"+unit+string(rune('0'+extra)), func(t *testing.T) {
					title, description := "作品", ""
					var model, params, style *string
					value := strings.Repeat(unit, field.max+extra)
					switch field.name {
					case "title":
						title = value
					case "description":
						description = value
					case "model":
						model = &value
					case "params":
						params = &value
					case "style":
						style = &value
					}
					_, err := NewPublished(1, title, description, "/video.mp4", "/cover.png", model, params, style, nil, "")
					if extra == 0 && err != nil {
						t.Fatalf("上限内的字符被拒绝: %v", err)
					}
					if extra == 1 && !errors.Is(err, field.err) {
						t.Fatalf("超限错误 = %v, 期望 %v", err, field.err)
					}
				})
			}
		}
	}
}
