import { z } from 'zod';

export const LogLevelSchema = z.enum(['debug', 'info', 'warn', 'error', 'fatal']);
export type LogLevel = z.infer<typeof LogLevelSchema>;

export const LogEventSchema = z.object({
  id:                   z.string(),
  trace_id:             z.string().optional(),
  span_id:              z.string().optional(),
  timestamp:            z.string().datetime(),
  received_at:          z.string().datetime().optional(),
  service_name:         z.string(),
  severity:             LogLevelSchema,
  severity_text:        z.string().optional(),
  body:                 z.string(),
  resource_attributes:  z.record(z.unknown()).optional(),
  log_attributes:       z.record(z.unknown()).optional(),
  schema_url:           z.string().optional(),
});
export type LogEvent = z.infer<typeof LogEventSchema>;

export const LogsResponseSchema = z.object({
  data:           z.array(LogEventSchema),
  next_cursor:    z.string().nullable().optional(),
  query_time_ms:  z.number(),
});
export type LogsResponse = z.infer<typeof LogsResponseSchema>;

export const ProblemSchema = z.object({
  type:       z.string(),
  title:      z.string(),
  status:     z.number(),
  detail:     z.string().optional(),
  instance:   z.string().optional(),
  violations: z.array(z.object({ field: z.string(), message: z.string() })).optional(),
});
export type Problem = z.infer<typeof ProblemSchema>;

// Query parameters passed to the API
export interface LogsQueryParams {
  start_time:   string;   // RFC3339
  end_time:     string;   // RFC3339
  service_name?: string;
  level?:        LogLevel;
  keyword?:      string;
  cursor?:       string;
  limit?:        number;
}
