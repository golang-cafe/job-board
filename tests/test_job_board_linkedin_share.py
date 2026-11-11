import os


def test_linkedin_ugc_post_client_and_headers():
    base = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    go_code = open(os.path.join(base, "internal", "linkedin", "linkedin.go"), encoding="utf-8").read()
    assert "https://api.linkedin.com/v2/ugcPosts" in go_code
    assert "com.linkedin.ugc.ShareContent" in go_code
    assert "X-Restli-Protocol-Version" in go_code
    assert "shareMediaCategory" in go_code


def test_linkedin_async_task_handler_route():
    base = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    h_code = open(os.path.join(base, "internal", "handler", "linkedin_task.go"), encoding="utf-8").read()
    assert "/x/task/linkedin-share-jobs" in h_code
    assert "TriggerLinkedInShareJobsTask" in h_code


def test_linkedin_go_unit_test_present():
    base = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    t_code = open(os.path.join(base, "internal", "linkedin", "linkedin_test.go"), encoding="utf-8").read()
    assert "TestShareJobPostSendsRestliHeadersAndUGCPayload" in t_code
