import { OctokitResponse } from "@octokit/plugin-paginate-rest/dist-types/types";
import { RequestError } from "@octokit/types";
import { Octokit, App } from "octokit";
import { Router, Params } from "./router";
import { safeParseInt } from "./util";

const octokit = new Octokit({ auth: GITHUB_TOKEN });

const repo = {
  owner: "maxmcd",
  repo: "bramble",
};

const router = new Router();

router.POST("/job", async (request: Request, params: Params): Promise<Response> => {
  try {
    let body: JobRequest = await request.json();
    console.log(body);
    return new Response(`job started?`);
  } catch (error: any) {
    return new Response(error.message, { status: 400 });
  }
});

type JobRequest = { Package: string; Reference: string };

router.GET("/job/:id", async (request: Request, params: Params): Promise<Response> => {
  try {
    let resp = await octokit.request("GET /repos/{owner}/{repo}/actions/jobs/{job_id}", {
      job_id: safeParseInt(params.id),
      ...repo,
    });
    console.log(resp);
    return new Response(
      JSON.stringify({
        ID: params.id,
        Start: resp.data.started_at,
        End: resp.data.completed_at,
        Error: resp.data.conclusion === "success" ? null : resp.data.conclusion,
      }),
    );
  } catch (error: any) {
    const err = <RequestError>error;
    if (err.status == 404) {
      return new Response("not found", { status: 404 });
    }
    return new Response(err.toString(), { status: err.status });
  }
});

router.GET("/job/:id/logs", async (request: Request, params: Params): Promise<Response> => {
  try {
    let resp = await octokit.request("GET /repos/{owner}/{repo}/actions/jobs/{job_id}/logs", {
      job_id: safeParseInt(params.id),
      ...repo,
    });
    console.log(resp);
    return new Response(`Redirecting to ${resp.url}`, {
      headers: { location: resp.url || "" },
      status: 302,
    });
  } catch (error: any) {
    const err = <RequestError>error;
    if (err.status == 404) {
      return new Response("not found", { status: 404 });
    }
    return new Response(err.toString(), { status: err.status });
  }
});

export async function handleRequest(request: Request): Promise<Response> {
  return router.serve(request);
}
