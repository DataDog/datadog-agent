ManifestSource := spec.yaml
MarkdownFolder := markdown
SphinxFolder   := site
export UV_CACHE_DIR := /Users/antoine.gagniere/.claude/plugins/data/speky-speky/uvcache
export UV_PROJECT  := /Users/antoine.gagniere/.claude/plugins/cache/speky/speky/0.4.0

Run    := uv run
Speky  := $(Run) speky
Sphinx := $(Run) --group docs sphinx-build

html: $(SphinxFolder)/html/index.html

open: html
	open $(SphinxFolder)/html/index.html

clean:
	$(RM) -r $(SphinxFolder) $(MarkdownFolder)

check:
	$(Speky) $(ManifestSource) --check-only

SpecSources := $(shell find . -name '*.yaml' -o -name '*.toml')

$(MarkdownFolder)/index.md: $(SpecSources)
	$(Speky) $(ManifestSource) --output-folder $(@D)

$(SphinxFolder)/html/index.html: $(MarkdownFolder)/index.md conf.py
	$(Sphinx) -M html $(MarkdownFolder) $(SphinxFolder) --conf-dir .

.PHONY: html open clean check
