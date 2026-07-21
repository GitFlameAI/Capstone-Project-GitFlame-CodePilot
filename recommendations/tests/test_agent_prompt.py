from agent_engine.prompt import SYSTEM_PROMPT


def test_agent_prompt_defines_empty_and_unavailable_rag_behavior():
    assert 'status: "empty"' in SYSTEM_PROMPT
    assert "RAG is unavailable or not configured" in SYSTEM_PROMPT
    assert "supplied repository files" in SYSTEM_PROMPT
    assert "TBD" in SYSTEM_PROMPT
    assert "invent additional paths" in SYSTEM_PROMPT
