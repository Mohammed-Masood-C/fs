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

  // Folder sizes are computed on demand, a few folders at a time, so the page
  // itself appears instantly even when folders are huge.
  const units = ["KB", "MB", "GB", "TB", "PB", "EB"];
  function humanSize(n) {
    if (n < 1024) return n + " B";
    let i = -1;
    do {
      n /= 1024;
      i++;
    } while (n >= 1024 && i < units.length - 1);
    return n.toFixed(1) + " " + units[i];
  }

  const sizeCells = Array.from(document.querySelectorAll("td[data-size]"));
  const sizeRequests = new AbortController();
  window.addEventListener("pagehide", () => sizeRequests.abort());

  async function loadSizes() {
    for (let cell = sizeCells.shift(); cell; cell = sizeCells.shift()) {
      try {
        const res = await fetch(cell.dataset.size, { signal: sizeRequests.signal });
        if (!res.ok) throw new Error("HTTP " + res.status);
        const { bytes, files } = await res.json();
        cell.textContent = humanSize(bytes);
        cell.title = `${bytes.toLocaleString()} bytes in ${files.toLocaleString()} files`;
      } catch {
        if (sizeRequests.signal.aborted) return;
        cell.textContent = "—";
        cell.title = "Size unavailable";
      }
    }
  }

  for (const cell of sizeCells) cell.textContent = "…";
  for (let i = 0; i < 4; i++) loadSizes();

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
