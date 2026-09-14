export interface User {
  id: string;
  email: string;
  name: string;
  created_at: string;
}

export interface Project {
  id: string;
  team_id: string;
  name: string;
  description: string | null;
  status: 'active' | 'archived';
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface Task {
  id: string;
  project_id: string;
  title: string;
  description: string | null;
  status: 'todo' | 'in_progress' | 'done';
  assignee_id: string | null;
  created_by: string;
  position: number;
  created_at: string;
  updated_at: string;
}

export interface Comment {
  id: string;
  task_id: string;
  author_id: string;
  author_name: string;
  body: string;
  created_at: string;
  updated_at: string;
}

export interface PaginatedMeta {
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}

export interface PaginatedTasks {
  items: Task[];
  meta: PaginatedMeta;
}

export interface TeamMembership {
  team_id: string;
  team_name: string;
  team_slug: string;
  role: 'owner' | 'member' | 'viewer';
}

export interface Attachment {
  id: string;
  task_id: string;
  filename: string;
  content_type: string;
  size_bytes: number;
  s3_url: string;
  uploaded_by: string;
  created_at: string;
}

export interface AnalyticsOverview {
  total_projects: number;
  total_tasks: number;
  tasks_by_status: Record<string, number>;
  total_comments: number;
  total_teams: number;
}

export interface Webhook {
  id: string;
  team_id: string;
  url: string;
  events: string[];
  active: boolean;
  created_at: string;
}

export interface TokenResponse {
  access_token: string;
  token_type: string;
  user: User;
}

export interface APIError {
  error: {
    code: string;
    message: string;
    details: Record<string, unknown>;
  };
}

export interface ListTasksParams {
  page?: number;
  page_size?: number;
  status?: Task['status'];
}

export class MeridianClient {
  constructor(
    private baseUrl: string,
    private getToken: () => string | null = () => null,
  ) {}

  private async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const token = this.getToken();
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(options.headers as Record<string, string>),
    };
    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }

    const response = await fetch(`${this.baseUrl}${path}`, {
      ...options,
      headers,
    });

    if (!response.ok) {
      const error = (await response.json()) as APIError;
      throw new Error(error.error?.message ?? 'Request failed');
    }

    if (response.status === 204) {
      return undefined as T;
    }

    return response.json() as Promise<T>;
  }

  login(email: string, password: string) {
    return this.request<TokenResponse>('/api/v1/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    });
  }

  register(email: string, name: string, password: string) {
    return this.request<TokenResponse>('/api/v1/auth/register', {
      method: 'POST',
      body: JSON.stringify({ email, name, password }),
    });
  }

  listMyTeams() {
    return this.request<TeamMembership[]>('/api/v1/me/teams');
  }

  getFeatureFlags() {
    return this.request<{ flags: Record<string, boolean> }>('/api/v1/feature-flags');
  }

  listProjects() {
    return this.request<Project[]>('/api/v1/projects');
  }

  getProject(projectId: string) {
    return this.request<Project>(`/api/v1/projects/${projectId}`);
  }

  listTasks(projectId: string, params: ListTasksParams = {}) {
    const query = new URLSearchParams();
    if (params.page) query.set('page', String(params.page));
    if (params.page_size) query.set('page_size', String(params.page_size));
    if (params.status) query.set('status', params.status);
    const qs = query.toString();
    return this.request<PaginatedTasks>(
      `/api/v1/projects/${projectId}/tasks${qs ? `?${qs}` : ''}`,
    );
  }

  getTask(taskId: string) {
    return this.request<Task>(`/api/v1/tasks/${taskId}`);
  }

  updateTask(taskId: string, data: Partial<Pick<Task, 'title' | 'description' | 'status' | 'assignee_id' | 'position'>>) {
    return this.request<Task>(`/api/v1/tasks/${taskId}`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    });
  }

  createTask(projectId: string, data: { title: string; description?: string; status?: Task['status']; assignee_id?: string }) {
    return this.request<Task>(`/api/v1/projects/${projectId}/tasks`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  listComments(taskId: string) {
    return this.request<Comment[]>(`/api/v1/tasks/${taskId}/comments`);
  }

  createComment(taskId: string, body: string) {
    return this.request<Comment>(`/api/v1/tasks/${taskId}/comments`, {
      method: 'POST',
      body: JSON.stringify({ body }),
    });
  }

  listAttachments(taskId: string) {
    return this.request<Attachment[]>(`/api/v1/tasks/${taskId}/attachments`);
  }

  createAttachment(taskId: string, data: { filename: string; content_type: string; size_bytes: number }) {
    return this.request<Attachment>(`/api/v1/tasks/${taskId}/attachments`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  getAnalyticsOverview() {
    return this.request<AnalyticsOverview>('/api/v1/analytics/overview');
  }

  listWebhooks() {
    return this.request<Webhook[]>('/api/v1/webhooks');
  }

  createWebhook(data: { team_id: string; url: string; events?: string[] }) {
    return this.request<Webhook>('/api/v1/webhooks', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }
}