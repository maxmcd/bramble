import { Router, Params } from "./router";

import { makeID } from "./util";
import { RequestError } from "@octokit/types";
import { Octokit } from "octokit";
import { safeParseInt, wrapError } from "./util";

const repo = {
  owner: "maxmcd",
  repo: "bramble",
};

type JobRequest = { Package?: string; Reference?: string } | undefined;
type Job = { RunID?: number; Start: string };
type RegisterJobRequest = { RunID?: number } | undefined;

export class BuilderWorker {
  value: number = 0;
  state: DurableObjectState;
  router: Router;
  octokit: Octokit;

  constructor(state: DurableObjectState, env: Env) {
    this.state = state;
    this.octokit = new Octokit({ auth: env.GITHUB_TOKEN });
    this.router = new Router();
    this.router.POST("/job", this.startJob.bind(this));
    this.router.POST("/job/:id/register", this.registerJob.bind(this));
    this.router.GET("/job/:id", this.getJobStatus.bind(this));
    this.router.GET("/job/:id/logs", this.getJobLogs.bind(this));
  }

  // Handle HTTP requests from clients.
  async fetch(request: Request) {
    return this.router.serve(request);
  }

  async getJob(id: string): Promise<Job | undefined> {
    return await this.state.storage?.get<Job>(id);
  }
  async newJob(): Promise<[string, Job]> {
    let id = "";
    while (true) {
      id = makeID();
      if (!(await this.state.storage?.get(id))) {
        break;
      }
    }
    const job = { Start: new Date().toISOString() };
    await this.state.storage?.put(id, job);
    return [id, job];
  }
  async updateJob(id: string, job: Job) {
    await this.state.storage?.put(id, job);
  }

  async startJob(request: Request, params: Params): Promise<Response> {
    let body: JobRequest;
    try {
      body = await request.json();
    } catch (err) {
      return new Response(err.message, { status: 400 });
    }
    if (!body || !body.Package) {
      return new Response("JSON body must include a 'Package' value.", { status: 400 });
    }
    let [id, job] = await this.newJob();
    try {
      await this.octokit.request(
        "POST /repos/{owner}/{repo}/actions/workflows/{workflow_id}/dispatches",
        {
          owner: "maxmcd",
          repo: "bramble",
          workflow_id: ".github/workflows/builder.yml",
          ref: "infra-2",
          inputs: { ...body, JobID: id },
        },
      );
    } catch (err) {
      return new Response(`Error trigging build with github: ${err.toString()}`, { status: 500 });
    }
    return new Response(id);
  }
  async registerJob(request: Request, params: Params): Promise<Response> {
    let body: RegisterJobRequest;
    try {
      body = await request.json();
    } catch (error) {
      return new Response(error.message, { status: 400 });
    }
    if (!body || !body.RunID) {
      return new Response("JSON body is not the expected format", { status: 400 });
    }
    let job = await this.getJob(params.id);
    if (job == null || job.RunID != null) {
      return new Response(`Job with id ${params.id} not found or not registerable`, {
        status: 400,
      });
    }
    job.RunID = body.RunID;
    await this.updateJob(params.id, job);
    return new Response("");
  }
  async _getJobStatus(jobID: string): Promise<{ job?: Job; runJobs?: any; resp?: Response }> {
    // Try to find the job, if we don't have a job with a runID then try and
    // find the job start
    let job = await this.getJob(jobID);

    if (job == null) {
      return { resp: new Response("not found, wat", { status: 404 }) };
    }
    if (job.RunID == null) {
      return {
        resp: new Response(
          JSON.stringify({
            ID: jobID,
            Start: job.Start,
            End: null,
            Error: null,
          }),
        ),
      };
    }

    try {
      let resp = await this.octokit.request(
        "GET /repos/{owner}/{repo}/actions/runs/{run_id}/attempts/{attempt_number}/jobs",
        {
          run_id: job.RunID,
          attempt_number: 1,
          ...repo,
        },
      );
      return { job: job, runJobs: resp.data };
    } catch (error) {
      const err = <RequestError>error;
      if (err.status == 404) {
        return {
          resp: new Response(`not found jobs ${jobID} ${job.RunID}`, { status: 404 }),
        };
      }
      return { resp: new Response(err.toString(), { status: err.status }) };
    }
  }
  async getJobStatus(request: Request, params: Params): Promise<Response> {
    let { job, runJobs, resp } = await this._getJobStatus(params.id);
    if (resp) {
      return resp;
    }
    return new Response(
      JSON.stringify({
        ID: params.id,
        j: runJobs,
        Start: job?.Start,
        End: runJobs.jobs[0].completed_at,
        Error: runJobs.jobs[0].conclusion === "success" ? null : runJobs.jobs[0].conclusion,
      }),
    );
  }
  async getJobLogs(request: Request, params: Params): Promise<Response> {
    let { job, runJobs, resp } = await this._getJobStatus(params.id);
    if (resp) return resp;
    try {
      let resp = await this.octokit.request(
        "GET /repos/{owner}/{repo}/actions/jobs/{job_id}/logs",
        {
          job_id: runJobs.jobs[0].id,
          ...repo,
        },
      );
      return new Response(`Redirecting to ${resp.url}`, {
        headers: { location: resp.url || "" },
        status: 302,
      });
    } catch (error) {
      const err = <RequestError>error;
      if (err.status == 404) {
        return new Response("not found", { status: 404 });
      }
      return new Response("Error from github logs endpoint: " + err.toString(), {
        status: err.status,
      });
    }
  }
}

interface Env {
  GITHUB_TOKEN: string;
}
