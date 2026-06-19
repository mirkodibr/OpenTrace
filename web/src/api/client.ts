import { LogsQueryParams, LogsResponse, LogsResponseSchema, Problem } from '@/types/api';

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly problem: Problem,
  ) {
    super(problem.detail ?? problem.title);
    this.name = 'ApiError';
  }
}

async function parseResponse<T>(res: Response, parse: (data: unknown) => T): Promise<T> {
  const contentType = res.headers.get('content-type') ?? '';
  const body = contentType.includes('json') ? await res.json() : await res.text();

  if (!res.ok) {
    throw new ApiError(res.status, body as Problem);
  }
  return parse(body);
}

export async function fetchLogs(params: LogsQueryParams): Promise<LogsResponse> {
  const query = new URLSearchParams();
  query.set('start_time', params.start_time);
  query.set('end_time', params.end_time);
  if (params.service_name) query.set('service_name', params.service_name);
  if (params.level)        query.set('level', params.level);
  if (params.keyword)      query.set('keyword', params.keyword);
  if (params.cursor)       query.set('cursor', params.cursor);
  if (params.limit)        query.set('limit', String(params.limit));

  const res = await fetch(`/api/v1/logs?${query.toString()}`);
  return parseResponse(res, (d) => LogsResponseSchema.parse(d));
}
