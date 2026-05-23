// api.jsx — central data hooks for the desktop shell.
//
// Depends on: React (already loaded by DevCore.app.html).
// Depended on by: app.jsx and views.jsx, which consume the exported hooks
//   (useApi, useStats, useTasks, useEvents, useCanonical, useCanonicalDoc)
//   in place of their original mock data.
// Why it exists: the views originated as a Claude-Design prototype with
//   hardcoded fixtures. The Swift shell launches devcore-api and passes its
//   base URL on the page's query string (`?api=http://127.0.0.1:<port>`).
//   This module reads that URL, fetches DevCore's real state, and falls back
//   to `null` when the API is not reachable so the views can render
//   placeholders rather than crashing.

(function () {
  // ─── API base URL ────────────────────────────────────────────────────
  // Read once at load time. The Swift shell sets `?api=` exactly once when it
  // builds the entry-point URL — no in-page navigation rewrites it.
  function readAPIBase() {
    try {
      const params = new URLSearchParams(window.location.search);
      const raw = params.get('api');
      if (!raw) return null;
      // Reject any value that does not look like a localhost URL — the page
      // origin is `devcore://`, so anything else would be cross-origin to a
      // host this app has no business talking to.
      if (!/^https?:\/\/(127\.0\.0\.1|localhost)(:\d+)?\/?$/.test(raw)) return null;
      return raw.replace(/\/$/, '');
    } catch (_e) {
      return null;
    }
  }

  const API_BASE = readAPIBase();

  // useApi exposes a stable view of the API base URL and a reachability flag
  // so components can render "live" vs "placeholder" badges without each one
  // re-reading window.location.
  function useApi() {
    return React.useMemo(() => ({ base: API_BASE, live: API_BASE !== null }), []);
  }

  // ─── Internal fetch + polling helper ─────────────────────────────────
  // useResource(path, intervalMs?) returns { data, error, loading } where
  // data is null until the first response arrives. When API_BASE is null the
  // hook returns a stable { data: null, error: null, loading: false } so
  // views can detect the offline case and render placeholder content.
  function useResource(path, intervalMs) {
    const [state, setState] = React.useState({
      data: null,
      error: null,
      loading: API_BASE !== null,
    });

    React.useEffect(() => {
      if (API_BASE === null) return undefined;
      let cancelled = false;

      const tick = async () => {
        try {
          const res = await fetch(API_BASE + path, { method: 'GET' });
          if (!res.ok) throw new Error('HTTP ' + res.status);
          const body = await res.json();
          if (!cancelled) setState({ data: body, error: null, loading: false });
        } catch (err) {
          if (!cancelled) setState({ data: null, error: err.message || String(err), loading: false });
        }
      };

      tick();
      let id;
      if (intervalMs && intervalMs > 0) {
        id = window.setInterval(tick, intervalMs);
      }
      return () => {
        cancelled = true;
        if (id) window.clearInterval(id);
      };
    }, [path, intervalMs]);

    return state;
  }

  // ─── Public hooks ────────────────────────────────────────────────────
  // useStats polls /api/stats every five seconds — slow enough to be cheap,
  // fast enough that the sidebar feels alive.
  function useStats() {
    return useResource('/api/stats', 5000);
  }

  // useTasks fetches the full task list, refreshed every ten seconds.
  function useTasks() {
    return useResource('/api/tasks', 10000);
  }

  // useEvents fetches the most recent 200 events, refreshed every five
  // seconds. The default page size matches devcore-api's defaultEventLimit.
  function useEvents(limit) {
    const path = '/api/events' + (limit ? '?limit=' + limit : '');
    return useResource(path, 5000);
  }

  // useCanonical fetches the list of canonical .md files — refreshes slowly
  // because the file tree changes infrequently relative to the event log.
  function useCanonical() {
    return useResource('/api/canonical', 30000);
  }

  // useCanonicalDoc fetches one canonical document by path. Pass an empty
  // path to skip the fetch entirely — useful for views that conditionally
  // load a selected file.
  function useCanonicalDoc(path) {
    const target = path ? '/api/canonical?path=' + encodeURIComponent(path) : '';
    return useResource(target, 0);
  }

  // Expose on the global so the script-tag-included view files can use them
  // without an import statement.
  window.DevCoreAPI = {
    base: API_BASE,
    useApi,
    useStats,
    useTasks,
    useEvents,
    useCanonical,
    useCanonicalDoc,
  };
})();
