import re

# A tool-output "signature": a KB document name, or a ticket/PR/issue reference produced by
# one of the agent's MCP tools (Linear / GitHub / Jira). Plain prose without any of these did
# not visibly come from a tool.
_TOOL_SIG = re.compile(r"\.md\b|FDE-\d+|PR\s*#?\d+|\bissue[s]?\b|\blinear\b|\bgithub\b|\bjira\b", re.IGNORECASE)

# The gold reference for off-topic / unknown-product rows DESCRIBES the expected behaviour
# ("the agent should state plainly it has no information", "la question est hors sujet ...").
# We treat those rows as not-applicable for tool use. EN + FR markers.
_NO_ANSWER = re.compile(
    r"no information|no such|not in the knowledge base|cannot find|must not invent|"
    r"hors sujet|rien (à|a) ce sujet|n'a rien|aucune information|ne doit pas inventer|"
    r"should (state|decline)|doit indiquer",
    re.IGNORECASE,
)

def evaluate(log):
    answer = str(log.get("output") or "")
    reference = str(log.get("reference") or "")
    retrievals = log.get("retrievals") or []
    # Off-topic / unknown -> declining is correct, no tool needed -> not applicable -> pass.
    if _NO_ANSWER.search(reference):
        return True
    tool_fired = bool(retrievals) or bool(_TOOL_SIG.search(answer))
    return tool_fired
