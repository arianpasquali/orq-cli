import re

# A citation marker a reader could follow: a KB document, a defect/PR/issue id, or a named
# source system. Plain arrows/dashes are intentionally NOT markers — only real sources.
_CITATION = re.compile(r"\.md\b|FDE-\d+|PR\s*#?\d+|\bissue[s]?\b|\blinear\b|\bgithub\b|\bjira\b", re.IGNORECASE)

# No-answer rows: the gold reference describes a decline ("no information ...", "hors sujet ...").
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
    # No-answer / off-topic row -> nothing to cite -> not applicable -> pass.
    if _NO_ANSWER.search(reference):
        return True
    cited = bool(retrievals) or bool(_CITATION.search(answer))
    return cited
