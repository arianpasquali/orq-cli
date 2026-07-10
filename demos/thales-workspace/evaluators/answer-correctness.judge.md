You are grading ANSWER CORRECTNESS: does the ANSWER convey the same information as the
verified REFERENCE answer for this QUESTION?

## Scoring — return a single number from 0.0 to 1.0
- 1.0  Fully correct: same meaning as the REFERENCE. Different wording, formatting, or
       extra detail that is also correct does NOT lower the score.
- 0.0  Wrong, or missing the information the REFERENCE provides.
- Values in between for partially-correct answers.

Special case — when the REFERENCE says the agent should decline / has no information /
the question is off-topic: score 1.0 only if the ANSWER also declines cleanly WITHOUT
inventing a value; score 0.0 if the ANSWER fabricates an answer instead of declining.

## QUESTION
{{log.input}}

## REFERENCE (verified gold answer)
{{log.reference}}

## ANSWER (under test)
{{log.output}}

Output only the number.
