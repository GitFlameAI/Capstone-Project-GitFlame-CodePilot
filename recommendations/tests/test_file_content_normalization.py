import pytest

from agent_engine.errors import InvalidGeneratedFilesError
from agent_engine.service import (
    _normalize_plain_file_content,
    _validate_modify_content_shape,
)


def test_plain_file_content_is_preserved_and_newline_terminated():
    assert _normalize_plain_file_content("src/app.py", "print('ok')") == "print('ok')\n"


@pytest.mark.parametrize("language", ["python", "py", ""])
def test_single_matching_outer_fence_is_removed(language):
    value = f"```{language}\nfrom pathlib import Path\n\nVALUE = Path('.')\n```"

    assert _normalize_plain_file_content("workers/shared_utils.py", value) == (
        "from pathlib import Path\n\nVALUE = Path('.')\n"
    )


def test_matching_fence_may_have_outer_whitespace():
    value = "\n  ```python\nprint('ok')\n```  \n"

    assert _normalize_plain_file_content("app.py", value) == "print('ok')\n"


@pytest.mark.parametrize(
    "value, expected_error",
    [
        (
            "Here is the file:\n```python\nprint('ok')\n```",
            "Markdown fences instead of plain file content",
        ),
        (
            "```javascript\nprint('ok')\n```",
            "javascript Markdown fence for app.py",
        ),
        (
            "```python title=app.py\nprint('ok')\n```",
            "unsupported Markdown fence declaration",
        ),
        (
            "```python\nvalue = '''```'''\n```",
            "nested or ambiguous Markdown fences",
        ),
        ("```python\n\n```", "empty file content"),
    ],
)
def test_ambiguous_or_mismatched_fences_are_rejected(value, expected_error):
    with pytest.raises(InvalidGeneratedFilesError, match=expected_error):
        _normalize_plain_file_content("app.py", value)


def test_unchanged_modify_content_is_rejected_for_retry():
    original = "def value():\n    return 1\n"

    with pytest.raises(InvalidGeneratedFilesError, match="unchanged from the original"):
        _validate_modify_content_shape("app.py", original, original)


def test_modify_content_with_real_change_is_accepted():
    _validate_modify_content_shape(
        "app.py",
        "def value():\n    return 2\n",
        "def value():\n    return 1\n",
    )
