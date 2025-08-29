# go-preloader-http

Preload API data for websites on the first fetch — a lightweight alternative to full server-side rendering (SSR).

Example project: [go-preloader-example](https://github.com/picklejw/go-preloader-example)

---

## Overview

Server-side rendering (SSR) has its advantages, mainly by reducing latency for initial page loads. Typically, when a client requests a page:

1. The browser fetches the HTML.
2. It then fetches additional resources (.js, .css, images).
3. Finally, JavaScript executes to fetch specific data for the requested page (e.g., product details).

SSR optimizes this by combining steps 1 and 3, sending a fully rendered HTML page with data already embedded. However, SSR is resource-intensive and requires careful management of server resources and styles. We want requests to be finished as quickly as possible so other requests can be served and devops events for triggering additional deployemnts to handle the additional load can be advoided if possible.

`go-preloader-http` provides a simpler, Go-based alternative: preload essential API data for your pages so that the client can render immediately without waiting for a secondary API call. It doesn’t replace SSR entirely but focuses on delivering the core data efficiently, making the best use case for SSR to be devices that are loading a webpage without JavaScript...

---

## How it Works

1. Identify the routes you want to preload data for (e.g., `/item?id="goat"`).
2. Register a corresponding API endpoint (e.g., `/api/item?id="goat"`) on the backend.
3. When a client requests `/item?id="goat`, the server bundles the API response into a `<script>` tag in the HTML with all paths that can be handled acending so this will also handle the data that a user might receive when requesting "/" which might have some user information for their valid session.
4. The frontend uses this preloaded data, avoiding an extra network request for the same endpoint. Subsequent requests without a page reload still work as normal via the API ( `/api/item?id="goat"` ) for when the user renders a new product on the page without a actual page reload.

This system aligns backend and frontend routes for seamless integration. It’s particularly useful if you:

* Funnel all `/api` requests to your backend.
* Serve static frontend builds from a specific root path (e.g., React build output). If the root path value is a empty string, this will default to making requests to `http://localhost:3000` which is what React uses to serve it's development build locally.

---

## Benefits

* **Lightweight and efficient**: Built in Go, reducing server resource usage compared to Node.js SSR. The same logic could be built in other languages to keep backend ecosystem unified.
* **Simplified workflow**: Frontend and backend teams only need route alignment; no complex inline CSS bundling required. Front End developers can use the module that will check if the data was bundled in `window.preloadHTTP` and if not then make a network request for the resources requested.
* **Reduced network calls**: Preloaded data eliminates the need for a first-fetch API request.
* **Easy to integrate**: Works with any static frontend, like React, without modifying existing build pipelines.

---

## Usage

```go
preloader := goPreloader.New(
    apiPrefix: "/api",
    reactAppBuildRoot: "./build",
)

// Register routes
preloader.RegisterRoute("/item/goat", func() interface{} {
    return getGoatData() // Replace with your data fetching logic
})
```

When a browser requests `/item/goat`:

* If the data is preloaded, it’s included in the HTML response.
* Otherwise, the frontend fetches `/api/item/goat` as usual.

---

`go-preloader-http` focuses on the core benefit of SSR — delivering page-specific data immediately — without the overhead of rendering full HTML on the server or bundling CSS/JS inline. It’s a practical, low-resource solution for modern web apps.
