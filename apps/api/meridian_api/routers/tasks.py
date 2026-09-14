import math
from typing import Annotated
from uuid import UUID, uuid4

from fastapi import APIRouter, Depends, Query
from psycopg2.extensions import connection

from meridian_api.auth import get_current_user
from meridian_api.database import get_db
from meridian_api.errors import APIError
from meridian_api.feature_flags import load_feature_flags
from meridian_api.policies import get_project_team_id, require_project_access, require_task_access
from meridian_api.schemas import PaginatedMeta, PaginatedTasksResponse, TaskCreate, TaskResponse, TaskUpdate
from meridian_api.webhook_queue import queue_webhook_event

router = APIRouter(tags=["tasks"])


def _queue_assignment_event(
    db: connection, task_id: UUID, assignee_id: UUID, assigned_by: UUID
) -> None:
    with db.cursor() as cur:
        cur.execute(
            """
            INSERT INTO task_assignment_events (id, task_id, assignee_id, assigned_by)
            VALUES (%s, %s, %s, %s)
            """,
            (str(uuid4()), str(task_id), str(assignee_id), str(assigned_by)),
        )


@router.get("/projects/{project_id}/tasks", response_model=PaginatedTasksResponse)
def list_tasks(
    project_id: UUID,
    db: Annotated[connection, Depends(get_db)],
    current_user: Annotated[dict, Depends(get_current_user)],
    page: int = Query(1, ge=1),
    page_size: int = Query(20, ge=1, le=100),
    status: str | None = Query(None, pattern="^(todo|in_progress|done)$"),
):
    require_project_access(db, current_user["id"], project_id, "viewer")

    conditions = ["t.project_id = %s", "tm.user_id = %s"]
    params: list = [str(project_id), str(current_user["id"])]

    if status:
        conditions.append("t.status = %s")
        params.append(status)

    where_clause = " AND ".join(conditions)

    with db.cursor() as cur:
        cur.execute(
            f"""
            SELECT COUNT(*) AS total
            FROM tasks t
            JOIN projects p ON p.id = t.project_id
            JOIN team_members tm ON tm.team_id = p.team_id
            WHERE {where_clause}
            """,
            params,
        )
        total = cur.fetchone()["total"]

        offset = (page - 1) * page_size
        cur.execute(
            f"""
            SELECT t.id, t.project_id, t.title, t.description, t.status,
                   t.assignee_id, t.created_by, t.position, t.created_at, t.updated_at
            FROM tasks t
            JOIN projects p ON p.id = t.project_id
            JOIN team_members tm ON tm.team_id = p.team_id
            WHERE {where_clause}
            ORDER BY t.position ASC, t.created_at ASC
            LIMIT %s OFFSET %s
            """,
            [*params, page_size, offset],
        )
        rows = cur.fetchall()

    return PaginatedTasksResponse(
        items=[TaskResponse(**row) for row in rows],
        meta=PaginatedMeta(
            page=page,
            page_size=page_size,
            total=total,
            total_pages=max(1, math.ceil(total / page_size)),
        ),
    )


@router.post("/projects/{project_id}/tasks", response_model=TaskResponse, status_code=201)
def create_task(
    project_id: UUID,
    body: TaskCreate,
    db: Annotated[connection, Depends(get_db)],
    current_user: Annotated[dict, Depends(get_current_user)],
):
    require_project_access(db, current_user["id"], project_id, "member")

    task_id = uuid4()
    with db.cursor() as cur:
        cur.execute(
            """
            INSERT INTO tasks (id, project_id, title, description, status, assignee_id, created_by, position)
            VALUES (%s, %s, %s, %s, %s, %s, %s, %s)
            RETURNING id, project_id, title, description, status, assignee_id,
                      created_by, position, created_at, updated_at
            """,
            (
                str(task_id),
                str(project_id),
                body.title,
                body.description,
                body.status,
                str(body.assignee_id) if body.assignee_id else None,
                str(current_user["id"]),
                body.position,
            ),
        )
        row = cur.fetchone()

    if body.assignee_id:
        _queue_assignment_event(db, task_id, body.assignee_id, current_user["id"])
        team_id = get_project_team_id(db, project_id)
        if team_id and load_feature_flags().get("webhook_integrations"):
            queue_webhook_event(
                db,
                team_id,
                "task.assigned",
                {"task_id": str(task_id), "assignee_id": str(body.assignee_id)},
            )

    return TaskResponse(**row)


@router.get("/tasks/{task_id}", response_model=TaskResponse)
def get_task(
    task_id: UUID,
    db: Annotated[connection, Depends(get_db)],
    current_user: Annotated[dict, Depends(get_current_user)],
):
    require_task_access(db, current_user["id"], task_id, "viewer")

    with db.cursor() as cur:
        cur.execute(
            """
            SELECT id, project_id, title, description, status, assignee_id,
                   created_by, position, created_at, updated_at
            FROM tasks WHERE id = %s
            """,
            (str(task_id),),
        )
        row = cur.fetchone()

    return TaskResponse(**row)


@router.patch("/tasks/{task_id}", response_model=TaskResponse)
def update_task(
    task_id: UUID,
    body: TaskUpdate,
    db: Annotated[connection, Depends(get_db)],
    current_user: Annotated[dict, Depends(get_current_user)],
):
    require_task_access(db, current_user["id"], task_id, "viewer")

    with db.cursor() as cur:
        cur.execute(
            """
            SELECT id, project_id, title, description, status, assignee_id,
                   created_by, position, created_at, updated_at
            FROM tasks WHERE id = %s
            """,
            (str(task_id),),
        )
        existing = cur.fetchone()

    updates = body.model_dump(exclude_unset=True)
    if not updates:
        return TaskResponse(**existing)

    set_clauses = []
    values = []
    for field, value in updates.items():
        set_clauses.append(f"{field} = %s")
        values.append(str(value) if field == "assignee_id" and value else value)

    values.append(str(task_id))

    with db.cursor() as cur:
        cur.execute(
            f"""
            UPDATE tasks SET {", ".join(set_clauses)}, updated_at = NOW()
            WHERE id = %s
            RETURNING id, project_id, title, description, status, assignee_id,
                      created_by, position, created_at, updated_at
            """,
            values,
        )
        row = cur.fetchone()

    if "assignee_id" in updates and updates["assignee_id"]:
        _queue_assignment_event(
            db, task_id, updates["assignee_id"], current_user["id"]
        )
        team_id = get_project_team_id(db, existing["project_id"])
        if team_id and load_feature_flags().get("webhook_integrations"):
            queue_webhook_event(
                db,
                team_id,
                "task.assigned",
                {"task_id": str(task_id), "assignee_id": str(updates["assignee_id"])},
            )

    if updates.get("status") == "done":
        team_id = get_project_team_id(db, existing["project_id"])
        if team_id and load_feature_flags().get("webhook_integrations"):
            queue_webhook_event(
                db,
                team_id,
                "task.completed",
                {"task_id": str(task_id)},
            )

    return TaskResponse(**row)


@router.delete("/tasks/{task_id}", status_code=204)
def delete_task(
    task_id: UUID,
    db: Annotated[connection, Depends(get_db)],
    current_user: Annotated[dict, Depends(get_current_user)],
):
    require_task_access(db, current_user["id"], task_id, "viewer")

    with db.cursor() as cur:
        cur.execute("DELETE FROM tasks WHERE id = %s", (str(task_id),))