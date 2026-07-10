import re

# Short, high-frequency words carry no retrieval signal — drop them so the recall ratio
# reflects content terms (product names, part numbers, defect ids, ...). EN + FR stop-words.
_STOP = {
    "the", "a", "an", "of", "to", "and", "or", "in", "on", "for", "with", "is", "are",
    "was", "were", "be", "been", "it", "its", "as", "at", "by", "that", "this", "from",
    "le", "la", "les", "des", "un", "une", "et", "ou", "de", "du", "dans", "pour", "avec",
    "est", "sont", "que", "qui", "sur", "aux", "ses", "son", "par", "une",
}

_NO_ANSWER = re.compile(
    r"no information|no such|not in the knowledge base|cannot find|must not invent|"
    r"hors sujet|rien (à|a) ce sujet|n'a rien|aucune information|ne doit pas inventer|"
    r"should (state|decline)|doit indiquer",
    re.IGNORECASE,
)

def _terms(text):
    toks = re.findall(r"[A-Za-zÀ-ÿ0-9]{3,}", (text or "").lower())
    return {t for t in toks if t not in _STOP}

def evaluate(log):
    reference = str(log.get("reference") or "")
    output = str(log.get("output") or "")
    retrievals = log.get("retrievals") or []
    # No-answer rows: nothing should have been retrieved -> not applicable -> 1.0.
    if _NO_ANSWER.search(reference):
        return 1.0
    gold = _terms(reference)
    if not gold:
        return 1.0
    if isinstance(retrievals, (list, tuple)):
        context = " ".join(str(x) for x in retrievals)
    else:
        context = str(retrievals)
    # Prefer measuring recall against the RETRIEVED context (true retrieval relevance);
    # fall back to the answer text when no retrievals are exposed.
    haystack = _terms(context) if context.strip() else _terms(output)
    covered = len(gold & haystack)
    return covered / len(gold)
