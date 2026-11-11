import os


def test_job_board_74_schema_and_audit_table():
    base = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    sql = open(os.path.join(base, "migrations", "001_developer_cv_upload.sql"), encoding="utf-8").read()
    assert "ADD COLUMN IF NOT EXISTS cv BYTEA" in sql
    assert "CREATE TABLE IF NOT EXISTS developer_cv_download" in sql
    assert "developer_profile_id" in sql
    assert "downloaded_at" in sql


def test_job_board_74_go_validation_and_recruiter_guard():
    base = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    go_code = open(os.path.join(base, "internal", "developer", "cv.go"), encoding="utf-8").read()
    assert "CV not uploaded" in go_code
    assert "%PDF-" in go_code
    assert "AuthorizeCVDownload" in go_code
    assert "RecordAndFetchDeveloperCVDownload" in go_code


def test_job_board_74_html_template_renders_cv_not_uploaded():
    base = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    html = open(
        os.path.join(base, "static", "views", "edit-developer-profile-cv.html"),
        encoding="utf-8",
    ).read()
    assert "CV not uploaded" in html
    assert 'accept="application/pdf"' in html
