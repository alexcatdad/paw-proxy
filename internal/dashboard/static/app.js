(function() {
  "use strict";

  var MAX_FEED = 200;
  var paused = false;
  var filterRoute = null;
  var pendingWhilePaused = [];

  var feedList = document.getElementById("feed-list");
  var pauseBtn = document.getElementById("pause-btn");
  var filterLabel = document.getElementById("filter-label");
  var filterNameEl = document.getElementById("filter-name");
  var clearFilter = document.getElementById("clear-filter");
  var sseDot = document.getElementById("sse-dot");
  var versionEl = document.getElementById("version");
  var uptimeEl = document.getElementById("uptime");
  var routesBody = document.getElementById("routes-body");
  var noRoutes = document.getElementById("no-routes");

  function fetchStats() {
    fetch("/api/stats")
      .then(function(r) { return r.json(); })
      .then(function(data) {
        versionEl.textContent = "v" + data.version;
        uptimeEl.textContent = "up " + data.uptime;
      })
      .catch(function() {});
  }

  function fetchRoutes() {
    fetch("/api/routes")
      .then(function(r) { return r.json(); })
      .then(function(routes) {
        routesBody.textContent = "";
        if (!routes || routes.length === 0) {
          noRoutes.hidden = false;
          return;
        }
        noRoutes.hidden = true;
        routes.forEach(function(route) {
          var tr = document.createElement("tr");
          tr.className = "clickable";
          tr.addEventListener("click", function() { setFilter(route.name); });

          var avgMs = route.requests > 0 ? Math.round(route.avgMs) : 0;

          var cells = [
            createLinkCell(route.name + ".test", "https://" + route.name + ".test"),
            createTextCell(route.upstream),
            createTextCell(shortenDir(route.dir)),
            createTextCell(formatUptime(route.registered)),
            createTextCell(String(route.requests)),
            createTextCell(avgMs + "ms"),
            createErrorCell(route.errors)
          ];
          cells.forEach(function(td) { tr.appendChild(td); });
          routesBody.appendChild(tr);
        });
      })
      .catch(function() {});
  }

  function createTextCell(text) {
    var td = document.createElement("td");
    td.textContent = text;
    return td;
  }

  function createLinkCell(text, href) {
    var td = document.createElement("td");
    var a = document.createElement("a");
    a.textContent = text;
    a.href = href;
    a.target = "_blank";
    td.appendChild(a);
    return td;
  }

  function createErrorCell(errors) {
    var td = document.createElement("td");
    td.textContent = String(errors);
    if (errors > 0) td.className = "errors-nonzero";
    return td;
  }

  function shortenDir(dir) {
    var home = "/Users/";
    var idx = dir.indexOf(home);
    if (idx === 0) {
      var rest = dir.substring(home.length);
      var slashIdx = rest.indexOf("/");
      if (slashIdx !== -1) {
        return "~" + rest.substring(slashIdx);
      }
    }
    return dir;
  }

  function formatUptime(registered) {
    var ms = Date.now() - new Date(registered).getTime();
    var s = Math.floor(ms / 1000);
    if (s < 60) return s + "s";
    var m = Math.floor(s / 60);
    if (m < 60) return m + "m";
    var h = Math.floor(m / 60);
    return h + "h " + (m % 60) + "m";
  }

  function statusClass(code) {
    if (code >= 500) return "status-5xx";
    if (code >= 400) return "status-4xx";
    if (code >= 300) return "status-3xx";
    return "status-2xx";
  }

  function formatTime(ts) {
    var d = new Date(ts);
    return d.toLocaleTimeString("en-US", { hour12: false });
  }

  function addFeedEntry(entry) {
    if (filterRoute && entry.route !== filterRoute) return;

    var div = document.createElement("div");
    div.className = "feed-entry";

    var parts = [
      { cls: "feed-time", text: formatTime(entry.timestamp) },
      { cls: "feed-method", text: entry.method },
      { cls: "feed-host", text: entry.host },
      { cls: "feed-path", text: entry.path },
      { cls: "feed-status " + statusClass(entry.statusCode), text: String(entry.statusCode) },
      { cls: "feed-latency", text: entry.latencyMs + "ms" }
    ];

    parts.forEach(function(p) {
      var span = document.createElement("span");
      span.className = p.cls;
      span.textContent = p.text;
      div.appendChild(span);
    });

    feedList.insertBefore(div, feedList.firstChild);

    while (feedList.children.length > MAX_FEED) {
      feedList.removeChild(feedList.lastChild);
    }
  }

  function connectSSE() {
    var es = new EventSource("/events");

    es.onopen = function() {
      sseDot.className = "dot dot-on";
      sseDot.title = "SSE connected";
    };

    es.onmessage = function(event) {
      var entry = JSON.parse(event.data);
      if (paused) {
        pendingWhilePaused.push(entry);
        if (pendingWhilePaused.length > MAX_FEED) pendingWhilePaused.shift();
        return;
      }
      addFeedEntry(entry);
    };

    es.onerror = function() {
      sseDot.className = "dot dot-off";
      sseDot.title = "SSE disconnected — reconnecting...";
    };
  }

  pauseBtn.addEventListener("click", function() {
    paused = !paused;
    pauseBtn.textContent = paused ? "Resume" : "Pause";
    if (!paused) {
      pendingWhilePaused.forEach(addFeedEntry);
      pendingWhilePaused = [];
    }
  });

  function setFilter(route) {
    filterRoute = route;
    filterNameEl.textContent = route + ".test";
    filterLabel.hidden = false;
    feedList.textContent = "";
  }

  clearFilter.addEventListener("click", function() {
    filterRoute = null;
    filterLabel.hidden = true;
    feedList.textContent = "";
  });

  fetchStats();
  fetchRoutes();
  connectSSE();
  setInterval(fetchRoutes, 5000);
  setInterval(fetchStats, 10000);
})();
