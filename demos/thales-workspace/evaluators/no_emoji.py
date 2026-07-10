import re

# Pictographic ranges + variation selector + regional indicators. Plain punctuation
# (arrows like -> , dashes) is intentionally NOT matched — only true emoji.
_EMOJI = re.compile(
    "["
    "\U0001F300-\U0001FAFF"   # symbols, pictographs, emoji, supplemental
    "\U00002600-\U000026FF"   # misc symbols (☀ ⚠ …)
    "\U00002700-\U000027BF"   # dingbats (✅ ✂ …)
    "\U0001F1E6-\U0001F1FF"   # regional indicators (flags)
    "\U00002B00-\U00002BFF"   # misc symbols & arrows (⭐ …)
    "\U0000FE0F"              # variation selector-16 (emoji presentation)
    "]"
)

def evaluate(log):
    text = log.get("output") if isinstance(log, dict) else log
    if not isinstance(text, str):
        text = str(text or "")
    return _EMOJI.search(text) is None
