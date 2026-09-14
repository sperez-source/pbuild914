"""Meridian CLI — admin and developer tooling."""

import typer

app = typer.Typer(name="meridian", help="Meridian admin and developer CLI")
tasks_app = typer.Typer(help="Task commands")
app.add_typer(tasks_app, name="tasks")


@tasks_app.command("export")
def export_tasks(
    project_id: str = typer.Option(..., "--project-id", help="Project UUID"),
):
    """Export tasks for a project as CSV to stdout."""
    # TODO: authenticate via MERIDIAN_TOKEN env var
    # TODO: call GET /api/v1/projects/{project_id}/tasks with pagination
    # TODO: print CSV header + rows
    typer.echo("Not implemented")
    raise typer.Exit(code=1)


if __name__ == "__main__":
    app()