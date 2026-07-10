import re

# French markers: accented characters, plus common French function / interrogative words.
_FR_ACCENTS = re.compile(r"[àâäéèêëîïôöùûüç]", re.IGNORECASE)
_FR_WORDS = re.compile(
    r"\b(le|la|les|des|une?|est|sont|avec|pour|dans|aucun|quelle?|comment|pourquoi|"
    r"recette|n'ai|trouvé|réduction)\b",
    re.IGNORECASE,
)

def _looks_french(text):
    text = text or ""
    return bool(_FR_ACCENTS.search(text)) or len(_FR_WORDS.findall(text)) >= 2

def evaluate(log):
    question = str(log.get("input") or "")
    answer = str(log.get("output") or "")
    want_french = _looks_french(question)   # expected language, inferred from the question
    answer_french = _looks_french(answer)
    return 1.0 if want_french == answer_french else 0.0
