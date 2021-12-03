import Route from "route-parser";

import { trimSuffix } from "./util";
export type Params = { [x: string]: string };
export type Handler = (request: Request, params: Params) => Promise<Response>;

export class Router {
  routes: Map<Route, [string, Handler]>;
  constructor() {
    this.routes = new Map();
  }
  any(route: string, handler: Handler, method?: string) {
    this.routes.set(new Route(route), [method || "", handler]);
  }
  GET(route: string, handler: Handler) {
    this.any(route, handler, "GET");
  }
  POST(route: string, handler: Handler) {
    this.any(route, handler, "POST");
  }
  async serve(request: Request): Promise<Response> {
    const url = new URL(request.url);
    for (let k of this.routes) {
      let [route, [method, handler]] = k;
      // See if path matches
      const params = route.match(trimSuffix(url.pathname, "/"));
      if (params) {
        // If we have a method and it doesn't match, respond with an error
        if (method !== "" && method !== request.method) {
          return new Response("method not allowed", { status: 405 });
        }
        // Otherwise return the handler
        return handler(request, params);
      }
    }
    return new Response("not found", { status: 404 });
  }
}
