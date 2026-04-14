/** MagiC Worker — registers capabilities and handles tasks via HTTP. */

import { createServer, type IncomingMessage, type ServerResponse } from "node:http";

/** Strip trailing slashes without regex (avoids ReDoS). */
function stripTrailingSlashes(s: string): string {
  let end = s.length;
  while (end > 0 && s[end - 1] === "/") end--;
  return s.slice(0, end);
}

interface WorkerOptions {
  name: string;
  endpoint: string;
  port?: number;
}

interface CapabilityEntry {
  name: string;
  description: string;
  handler: (input: Record<string, unknown>) => Promise<unknown>;
}

export class Worker {
  readonly name: string;
  readonly endpoint: string;
  private port: number;
  private capabilities = new Map<string, CapabilityEntry>();
  private workerId?: string;

  constructor(options: WorkerOptions) {
    this.name = options.name;
    this.endpoint = options.endpoint;
    this.port = options.port ?? this.parsePort(options.endpoint);
  }

  private parsePort(endpoint: string): number {
    const match = endpoint.match(/:(\d+)/);
    return match ? parseInt(match[1], 10) : 9000;
  }

  capability(name: string, description: string, handler: (input: Record<string, unknown>) => Promise<unknown>): void {
    this.capabilities.set(name, { name, description, handler });
  }

  async register(serverURL: string): Promise<void> {
    const payload = {
      name: this.name,
      capabilities: Array.from(this.capabilities.values()).map(({ name, description }) => ({ name, description })),
      endpoint: { type: "http", url: this.endpoint },
    };
    const res = await fetch(`${stripTrailingSlashes(serverURL)}/api/v1/workers/register`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      throw new Error(`Registration failed: ${res.status} ${await res.text()}`);
    }
    const data = (await res.json()) as { id?: string };
    this.workerId = data.id;
    console.log(`Registered as ${this.workerId} with ${this.capabilities.size} capabilities`);
  }

  serve(): void {
    const server = createServer(async (req: IncomingMessage, res: ServerResponse) => {
      if (req.method !== "POST") {
        res.writeHead(405).end();
        return;
      }

      let body: string;
      try {
        body = await this.readBody(req);
      } catch {
        res.writeHead(400).end("Bad request");
        return;
      }

      let msg: { type?: string; payload?: Record<string, unknown> };
      try {
        msg = JSON.parse(body);
      } catch {
        res.writeHead(400).end("Invalid JSON");
        return;
      }

      if (msg.type !== "task.assign") {
        res.writeHead(404).end(`Unknown message type: ${msg.type}`);
        return;
      }

      const payload = msg.payload ?? {};
      const taskId = (payload.task_id as string) ?? "unknown";
      const taskType = (payload.task_type as string) ?? "";
      const input = (payload.input as Record<string, unknown>) ?? {};

      let response: unknown;
      const cap = this.capabilities.get(taskType);
      if (!cap) {
        response = {
          type: "task.fail",
          payload: { task_id: taskId, error: { code: "no_handler", message: `No handler for ${taskType}` } },
        };
      } else {
        try {
          const result = await cap.handler(input);
          response = {
            type: "task.complete",
            payload: { task_id: taskId, output: typeof result === "string" ? { result } : result, cost: 0 },
          };
        } catch (e: unknown) {
          response = {
            type: "task.fail",
            payload: { task_id: taskId, error: { code: "handler_error", message: String(e) } },
          };
        }
      }

      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify(response));
    });

    server.listen(this.port, "0.0.0.0", () => {
      console.log(`${this.name} serving on 0.0.0.0:${this.port}`);
    });
  }

  private readBody(req: IncomingMessage): Promise<string> {
    return new Promise((resolve, reject) => {
      const chunks: Buffer[] = [];
      req.on("data", (chunk: Buffer) => chunks.push(chunk));
      req.on("end", () => resolve(Buffer.concat(chunks).toString()));
      req.on("error", reject);
    });
  }
}
