You are grading the FAITHFULNESS of an engineering assistant's ANSWER: does it only
state things its source material supports?

Source material, in priority order:
1. CONTEXT — the knowledge-base chunks retrieved to answer this question.
2. REFERENCE — the verified gold answer (use this when CONTEXT is empty or silent).

## Scoring — return a single number from 0.0 to 1.0
- 1.0  Every factual claim in the ANSWER is supported by the source material.
- 0.0  The ANSWER invents facts not in the source material, or contradicts it.
- Values in between for answers that are partly supported, partly invented.

Declining or saying "I don't know" asserts nothing, so it is FAITHFUL: score 1.0.
If BOTH the CONTEXT and the REFERENCE are empty, faithfulness is not applicable: score 1.0.

## QUESTION
{{log.input}}

## CONTEXT (retrieved knowledge-base chunks)
{{log.retrievals}}

## REFERENCE (verified gold answer)
{{log.reference}}

## ANSWER (under test)
{{log.output}}

Output only the number.
