/** MagiC HTTP client for communicating with the server. */

export interface TaskRequest {
  type: string;
  input: Record<string, unknown>;
  routing?: { strategy?: string; required_capabilities?: string[] };
  contract?: { timeout_ms?: number; max_cost?: number };
}

export interface Task {
  id: string;
  type: string;
  status: string;
  input: Record<string, unknown>;
  output?: Record<string, unknown>;
  error?: { code: string; message: string };
}

export interface WorkerInfo {
  id: string;
  name: string;
  status: string;
  capabilities: { name: string; description: string }[];
}

export interface WorkflowRequest {
  name: string;
  steps: {
    id: string;
    task_type: string;
    input: Record<string, unknown>;
    depends_on?: string[];
    on_failure?: string;
  }[];
}

export interface Workflow {
  id: string;
  name: string;
  status: string;
  steps: Record<string, unknown>[];
}

export class MagiCClient {
  private baseURL: string;
  private headers: Record<string, string>;

  constructor(baseURL: string, apiKey?: string) {
    this.baseURL = baseURL.replace(/\/+$/, "");
    this.headers = { "Content-Type": "application/json" };
    if (apiKey) {
      this.headers["Authorization"] = `Bearer ${apiKey}`;
    }
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const res = await fetch(`${this.baseURL}${path}`, {
      method,
      headers: this.headers,
      body: body ? JSON.stringify(body) : undefined,
    });
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`MagiC API error ${res.status}: ${text}`);
    }
    return res.json() as Promise<T>;
  }

  async submitTask(task: TaskRequest): Promise<Task> {
    return this.request("POST", "/api/v1/tasks", task);
  }

  async getTask(id: string): Promise<Task> {
    return this.request("GET", `/api/v1/tasks/${id}`);
  }

  async listWorkers(): Promise<WorkerInfo[]> {
    return this.request("GET", "/api/v1/workers");
  }

  async submitWorkflow(workflow: WorkflowRequest): Promise<Workflow> {
    return this.request("POST", "/api/v1/workflows", workflow);
  }

  async getWorkflow(id: string): Promise<Workflow> {
    return this.request("GET", `/api/v1/workflows/${id}`);
  }
}
