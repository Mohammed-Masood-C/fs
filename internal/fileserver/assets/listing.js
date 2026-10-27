(() => {
  "use strict";

  const filter = document.getElementById("filter");
  const noMatch = document.getElementById("nomatch");
  const rows = Array.from(document.querySelectorAll("#files tbody tr.row"));
  const names = rows.map((row) => (row.dataset.name || "").toLowerCase());
  const up = document.querySelector("#files tbody tr.up a.item");

  // Show modification times in the visitor's locale and time zone.
  const format = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" });
  for (const el of document.querySelectorAll("time[datetime]")) {
    const date = new Date(el.dateTime);
    if (!Number.isNaN(date.getTime())) {
      el.textContent = format.format(date);
      el.title = date.toString();
    }
  }

  // Filter the rows of the current page as the user types.
  function applyFilter() {
    const query = filter.value.trim().toLowerCase();
    let shown = 0;
    rows.forEach((row, i) => {
      const match = query === "" || names[i].includes(query);
      row.hidden = !match;
      if (match) shown++;
    });
    noMatch.hidden = query === "" || shown > 0 || rows.length === 0;
  }

  if (rows.length > 0) {
    filter.hidden = false;
    filter.addEventListener("input", applyFilter);
    // Browsers restore the input value when navigating back.
    window.addEventListener("pageshow", applyFilter);
  }

  // Links that keyboard navigation moves between, in order.
  function items() {
    const list = rows.filter((row) => !row.hidden).map((row) => row.querySelector("a.item"));
    return up ? [up, ...list] : list;
  }

  function moveFocus(delta) {
    const list = items();
    if (list.length === 0) return;
    const current = list.indexOf(document.activeElement);
    if (current === 0 && delta < 0 && !filter.hidden) {
      filter.focus();
      return;
    }
    const next = current < 0
      ? (delta > 0 ? 0 : list.length - 1)
      : Math.min(list.length - 1, Math.max(0, current + delta));
    list[next].focus();
    list[next].scrollIntoView({ block: "nearest" });
  }

  document.addEventListener("keydown", (event) => {
    if (event.defaultPrevented || event.ctrlKey || event.metaKey) return;
    const inFilter = event.target === filter;
    const inOtherField = !inFilter && event.target instanceof HTMLInputElement;
    if (inOtherField) return;

    switch (event.key) {
      case "/":
        if (!inFilter && !filter.hidden) {
          event.preventDefault();
          filter.focus();
          filter.select();
        }
        break;
      case "Escape":
        if (inFilter) {
          filter.value = "";
          applyFilter();
          filter.blur();
        }
        break;
      case "Enter":
        if (inFilter) {
          // Open the first match.
          const first = items().find((a) => a !== up);
          if (first) {
            event.preventDefault();
            window.location.href = first.href;
          }
        }
        break;
      case "ArrowDown":
        event.preventDefault();
        moveFocus(1);
        break;
      case "ArrowUp":
        if (event.altKey && up) {
          event.preventDefault();
          window.location.href = up.href;
        } else if (!inFilter) {
          event.preventDefault();
          moveFocus(-1);
        }
        break;
      case "Backspace":
        if (!inFilter && up) {
          event.preventDefault();
          window.location.href = up.href;
        }
        break;
    }
  });
})();
