//go:build darwin

package darwin

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func TestRenderLaunchdPlistUsesSingleBinaryCommand(t *testing.T) {
	content := renderLaunchdPlist(
		"com.example.ghost&gc",
		"/Applications/Ghost & Co/ghostgc",
		[]string{"daemon", "--config", "/tmp/config & audit.yaml"},
	)

	wantCommand := strings.Join([]string{
		"\t\t<string>/Applications/Ghost &amp; Co/ghostgc</string>",
		"\t\t<string>daemon</string>",
		"\t\t<string>--config</string>",
		"\t\t<string>/tmp/config &amp; audit.yaml</string>",
	}, "\n")
	if !strings.Contains(content, wantCommand) {
		t.Fatalf("ProgramArguments did not preserve the command order:\n%s", content)
	}
	for _, want := range []string{
		"<string>com.example.ghost&amp;gc</string>",
		"<key>StandardOutPath</key>\n\t<string>/dev/null</string>",
		"<key>StandardErrorPath</key>\n\t<string>/dev/null</string>",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("rendered plist missing %q", want)
		}
	}
	if strings.Contains(content, "ghostgc.out.log") || strings.Contains(content, "ghostgc.err.log") {
		t.Error("launchd must not write an unbounded service log")
	}
	if strings.Contains(content, "ghostgcd") {
		t.Error("rendered plist still references the removed daemon executable")
	}

	decoder := xml.NewDecoder(strings.NewReader(content))
	for {
		if _, err := decoder.Token(); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("rendered plist is not valid XML: %v", err)
		}
	}
}
