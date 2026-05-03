package openai

import (
	"testing"
)

func TestContainsGrokImageReference_BoundaryCases(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
		desc    string
	}{
		// --- 修复后：普通文本 grok.com 不应匹配 ---
		{
			name:    "conversation_mentions_grok.com",
			content: "Grok is a product at grok.com",
			want:    false,
			desc:    "用户问 grok.com，模型回复中提到域名 → 不应拦截",
		},
		{
			name:    "grok.com.evil.com_no_match",
			content: "visit grok.com.evil.com for images",
			want:    false,
			desc:    "grok.com.evil.com 不是 Grok 域名 → 不应拦截",
		},
		{
			name:    "grok.community_no_match",
			content: "visit grok.community for discussions",
			want:    false,
			desc:    "grok.community 域名不应匹配",
		},
		{
			name:    "bare_grok_com_with_trailing_slash",
			content: "see grok.com/ for more",
			want:    false,
			desc:    "bare grok.com/ 不应拦截（无 scheme）",
		},
		{
			name:    "bare_grok_com_at_sentence_end",
			content: "go to grok.com",
			want:    false,
			desc:    "grok.com 在句尾 → 不应拦截",
		},
		{
			name:    "bare_grok_com_in_parentheses",
			content: "(see grok.com)",
			want:    false,
			desc:    "grok.com 在括号中 → 不应拦截",
		},
		{
			name:    "bare_grok_com_with_colon_port",
			content: "grok.com:8080/api",
			want:    false,
			desc:    "grok.com:8080 不应拦截（无 scheme）",
		},
		{
			name:    "bare_subdomain_grok_com",
			content: "assets.grok.com is down",
			want:    false,
			desc:    "子域名 assets.grok.com 纯文本 → 不应拦截",
		},
		{
			name:    "subdomain_evil_grok_com",
			content: "evil.grok.com/phish",
			want:    false,
			desc:    "evil.grok.com 纯文本 → 不应拦截",
		},
		{
			name:    "word_grok_without_dot_com",
			content: "the grok system works well",
			want:    false,
			desc:    "单独 'grok' 无 .com 不应匹配",
		},

		// --- 修复后：需要路径模式匹配的绝对 URL ---
		{
			name:    "https_grok_com_about_non_image_path",
			content: "visit https://grok.com/about for details",
			want:    false,
			desc:    "https://grok.com/about 非媒体路径 → 不应拦截",
		},
		{
			name:    "https_grok_com_pricing",
			content: "https://grok.com/pricing",
			want:    false,
			desc:    "https://grok.com/pricing 非媒体路径 → 不应拦截",
		},
		{
			name:    "https_assets_grok_com_logo_svg",
			content: "https://assets.grok.com/logo.svg",
			want:    false,
			desc:    "assets.grok.com/logo.svg 非生成媒体路径 → 不应拦截",
		},
		{
			name:    "https_assets_grok_com_media_path",
			content: "https://assets.grok.com/users/x/generated/y.png",
			want:    true,
			desc:    "assets.grok.com 媒体路径 → 应拦截",
		},
		{
			name:    "https_grok_com_img_path",
			content: "https://grok.com/img/abc/1.png",
			want:    true,
			desc:    "grok.com/img 媒体路径 → 应拦截",
		},

		// --- 修复后：imagine-public 媒体子域名 ---
		{
			name:    "imagine_public_media_url",
			content: "https://imagine-public.grok.com/gen/abc",
			want:    true,
			desc:    "imagine-public.grok.com 是媒体专用子域名 → 应拦截",
		},
		{
			name:    "imagine_public_as_filename",
			content: "download the imagine-public-key.pem file",
			want:    false,
			desc:    "imagine-public-key.pem 不应拦截",
		},
		{
			name:    "imagine_public_in_sentence",
			content: "the imagine-public dataset is available",
			want:    false,
			desc:    "imagine-public 在普通句子中 → 不应拦截",
		},
		{
			name:    "imagine_private_no_match",
			content: "imagine-private-key.pem",
			want:    false,
			desc:    "imagine-private 不应匹配",
		},

		// --- relativeImagePathRe 场景（不变）---
		{
			name:    "relative_image_path_content",
			content: "users/abc123/file/content",
			want:    true,
			desc:    "users/*/content 路径 → 匹配",
		},
		{
			name:    "relative_image_path_generated",
			content: "users/abc/generated/image.png",
			want:    true,
			desc:    "users/*/generated/* 路径 → 匹配",
		},
		{
			name:    "content_as_plain_word",
			content: "the content of this message is important",
			want:    false,
			desc:    "content 作为普通单词 → 不应匹配 relativeImagePathRe",
		},
		{
			name:    "content_at_start",
			content: "content is king",
			want:    false,
			desc:    "content 在句首 → 不应匹配",
		},
		{
			name:    "users_without_content_or_generated",
			content: "users/abc/file/other",
			want:    false,
			desc:    "users 路径但无 content 或 generated → 不应匹配",
		},
		{
			name:    "users_word_in_sentence",
			content: "all users should read the content carefully",
			want:    false,
			desc:    "users 和 content 在普通句子中分散出现 → 不应匹配",
		},
		{
			name:    "user_content_no_slash_pattern",
			content: "user content is important",
			want:    false,
			desc:    "user content 无 users/*/ 前缀 → 不应匹配",
		},

		// --- 组合场景 ---
		{
			name:    "clean_normal_response",
			content: "The answer is 42. Have a great day!",
			want:    false,
			desc:    "正常回复 → 不应有任何匹配",
		},
		{
			name:    "code_with_grok_string",
			content: "func grok() { return 'hello' }",
			want:    false,
			desc:    "代码中 grok 函数名 → 不应匹配",
		},
		{
			name:    "markdown_non_grok_image",
			content: "![cat](https://example.com/cat.jpg)",
			want:    false,
			desc:    "非 grok 的 markdown 图片 → 不应匹配",
		},
		{
			name:    "markdown_grok_image",
			content: "![img](https://assets.grok.com/img.png)",
			want:    true,
			desc:    "grok 的 markdown 图片 → 匹配",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsGrokImageReference(tt.content)
			if got != tt.want {
				t.Errorf("containsGrokImageReference(%q) = %v, want %v\n  说明: %s",
					tt.content, got, tt.want, tt.desc)
			}
		})
	}
}
