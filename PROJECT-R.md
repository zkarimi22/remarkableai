# PROJECT-R

## Concept

PROJECT-R explores an AI-native operating system for an e-ink tablet, using the reMarkable as a paper-like interface for reading, writing, generating pages, and triggering actions.

The core interaction is:

1. The user writes or edits a page on the reMarkable.
2. The current page is sent to the AI as an image, together with any available document context.
3. The AI interprets printed text, handwriting, layout, and action symbols.
4. The AI converts the interpretation into structured actions.
5. External tools execute those actions.
6. The system generates an updated response page and sends it back to the reMarkable.

## Flow 1: Action prompts with designated symbols

The user can write an action prompt followed by a designated symbol. The symbol tells the system what kind of action to perform.

Example:

```text
Send an email to John:
"Hey John, great meeting yesterday. I'd love to catch up on your use cases
and schedule a follow up. Does Tues at 3pm PT work? Best, Zalmay"
*
```

Possible symbol registry:

```json
{
  "*": "send_email",
  "!": "create_ticket",
  "?": "research",
  "✓": "mark_complete",
  "@": "schedule_followup"
}
```

The AI should parse the page into structured intent, for example:

```json
{
  "action": "send_email",
  "recipient": "John",
  "body": "Hey John, great meeting yesterday. I'd love to catch up on your use cases and schedule a follow up. Does Tues at 3pm PT work? Best, Zalmay",
  "confirmation_required": true
}
```

Multiple action prompts may exist on the same page. The system should identify each prompt, associate it with the symbol on its right, execute the applicable actions, and generate a result page such as:

```text
ACTIONS COMPLETED

✓ Email sent to John
✓ Ticket created in the mobile project
✓ Follow-up reminder scheduled
```

Actions with external side effects, such as sending emails, deleting data, purchasing items, or creating public tickets, should support a review step. The AI can generate a confirmation page containing `APPROVE`, `EDIT`, and `CANCEL` options before execution.

## Flow 2: Generate pages and send them to the reMarkable

The system should be able to create visual pages from templates, variables, AI-generated content, or generated images.

Pipeline:

```text
Variables + template or image prompt
        ↓
Generated SVG or raster image
        ↓
PDF or PNG conversion
        ↓
Upload or sync to reMarkable
        ↓
Display as a page for reading and annotation
```

SVG should be used as an intermediate representation for layouts containing text, lines, boxes, diagrams, and vector graphics. Convert the SVG to PDF or PNG before sending it to the reMarkable. PDF is preferred for structured pages because it preserves layout and vector quality; PNG is useful for purely visual pages or generated artwork.

Example template:

```xml
<text x="80" y="120">Hello {{name}}</text>
<text x="80" y="180">{{meeting_summary}}</text>
<text x="80" y="260">Next step: {{next_action}}</text>
```

Example variables:

```json
{
  "name": "John",
  "meeting_summary": "Discussed onboarding and API access.",
  "next_action": "Schedule a follow-up next Tuesday."
}
```

Potential generated pages include:

- daily briefings populated from calendars, email, and tasks
- meeting summaries with action symbols
- personalized travel itineraries
- project dashboards
- AI-generated diagrams and flowcharts
- printable forms with writing areas and checkboxes
- storyboards and comic pages
- annotated maps and research pages
- study plans, workout plans, and meal plans
- visual control panels for external services

## Combined experience

The most important version combines both flows. The AI generates a page containing information and action prompts, the user annotates or marks the page physically, and the system interprets the updated page.

Example:

```text
PROJECT: MOBILE LOGIN

Investigate timeout reports              ?
Create GitHub issue                       !
Email Alex with findings                  *
Schedule review meeting                   @
```

The user can circle a prompt, check a box, underline text, add handwriting, or write additional parameters. After the AI interaction is triggered, the system reads the page, executes the selected actions, and generates a replacement page showing the results.

## Suggested architecture

```text
reMarkable page
      ↓
Page capture and document state
      ↓
Vision, OCR, handwriting, and ink-change interpretation
      ↓
Action parser and symbol registry
      ↓
Structured action plan with confidence scores
      ↓
Confirmation gate for risky actions
      ↓
External tool adapters
      ↓
Result renderer
      ↓
PDF or PNG page sent back to reMarkable
```

Each action should be represented by a tool definition with:

- action name
- designated symbol
- required fields
- optional fields
- external service or API
- confirmation policy
- result message format
- error handling behavior

Example action definition:

```json
{
  "name": "send_email",
  "symbol": "*",
  "required_fields": ["recipient", "body"],
  "confirmation": "required",
  "side_effect": "external_message"
}
```

## Initial implementation target

Build a narrow vertical slice before supporting many actions:

1. Define a single-page action format.
2. Support one symbol, such as `*`, for sending an email draft.
3. Capture the page image and identify the prompt text and symbol.
4. Return a structured JSON action.
5. Render a confirmation page as SVG.
6. Convert the SVG to PDF.
7. Send the PDF back to the reMarkable.
8. After approval, execute the email action and render an `Actions Completed` page.

Once this works end to end, add more symbols and tool adapters.

## Design principles

- Treat pages as both documents and user interfaces.
- Make symbols configurable rather than hard-coded into the model prompt.
- Preserve the relationship between text, symbols, and page position.
- Keep an audit trail of interpreted prompts and executed actions.
- Show the user what the AI understood before risky side effects.
- Make every generated page readable without requiring the AI or network connection.
- Preserve user annotations when generating updated pages where possible.

