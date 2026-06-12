package alfred

import (
	"strings"
	"testing"
)

func TestRenderWithRerun(t *testing.T) {
	items := []Item{{Title: "x"}}

	t.Run("zero interval omits the rerun field", func(t *testing.T) {
		out, err := RenderWithRerun(items, 0)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), "rerun") {
			t.Errorf("rerun should be omitted at 0: %s", out)
		}
	})

	t.Run("non-zero interval emits the rerun field", func(t *testing.T) {
		out, err := RenderWithRerun(items, 0.7)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), `"rerun":0.7`) {
			t.Errorf("expected rerun:0.7 in output: %s", out)
		}
	})
}
