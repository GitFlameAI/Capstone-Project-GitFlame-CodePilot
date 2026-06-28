class AgentEngineError(RuntimeError):
    """Base class for expected Agent Engine failures."""

    code = "agent_engine_error"


class ModelUnavailableError(AgentEngineError):
    code = "model_unavailable"


class RagUnavailableError(AgentEngineError):
    code = "rag_unavailable"


class InvalidPlanError(AgentEngineError):
    code = "invalid_output"


class InvalidGeneratedFilesError(AgentEngineError):
    code = "invalid_generated_files"


class EmptyModelOutputError(AgentEngineError):
    code = "empty_output"


class ToolLimitExceededError(AgentEngineError):
    code = "tool_limit_exceeded"


class InferenceTimeoutError(AgentEngineError):
    code = "inference_timeout"


class ToolExecutionError(AgentEngineError):
    code = "tool_execution_error"


class ConfigurationError(ValueError):
    code = "invalid_configuration"
