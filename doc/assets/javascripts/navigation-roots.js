(() => {
  let backLink = null;
  let backKey = "";

  document$.subscribe(() => {
    backLink = null;
    backKey = "";
    const root = document.querySelector("[data-navigation-root-back]");
    if (!root) return;

    backKey = root.dataset.navigationRootBackKey;
    const url = new URL(root.dataset.navigationRootBack, document.baseURI).href;
    for (const link of document.querySelectorAll("nav a[href]")) {
      if (link.href === url) {
        link.setAttribute("aria-keyshortcuts", backKey);
        backLink ??= link;
      }
    }
  });

  // A single listener survives instant navigation while the current link is refreshed above.
  document.addEventListener("keydown", (event) => {
    if (
      !backLink ||
      event.key !== backKey ||
      event.defaultPrevented ||
      event.repeat ||
      event.isComposing ||
      event.ctrlKey ||
      event.altKey ||
      event.metaKey ||
      event.shiftKey
    ) return;

    // Search inputs live in a shadow root, so the event target may be their host.
    if (event.composedPath().some((target) => target instanceof Element && (
      target.isContentEditable ||
      target.closest("input, textarea, select, [role='textbox'], [role='combobox']")
    ))) return;

    event.preventDefault();
    event.stopPropagation();
    backLink.click();
  });
})();
