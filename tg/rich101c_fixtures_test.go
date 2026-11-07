// Bot API 10.1 rich message fixtures (B-25 part 10.1c): the five sendRichMessage replies of a live probe on
// 2026-11-07, written by a script kept with the slice's review evidence (fixtures101c.py, not in this repository).
//
// Each constant is the reply's "result" (a tg.Message). Identity fields are replaced because the repository is
// public: message_id, date, chat id and title, the bot's id, first_name and username (internal/testutil values),
// and every file_id / file_unique_id (FILEID-<n> / UNIQ-<n>). The "rich_message" object is otherwise byte-exact.

package tg_test

const (
	// rich101cProbeInline: probe 01 (01-sendRichMessage-inline.json).
	rich101cProbeInline = `{
 "message_id": 1001,
 "from": {
  "id": 123456789,
  "is_bot": true,
  "first_name": "Test Bot",
  "username": "testbot"
 },
 "chat": {
  "id": -1001234567890,
  "title": "Test Chat",
  "is_forum": true,
  "type": "supergroup"
 },
 "date": 1700000000,
 "rich_message": {
  "blocks": [
   {
    "type": "paragraph",
    "text": [
     {
      "type": "bold",
      "text": "bold"
     },
     " ",
     {
      "type": "italic",
      "text": "italic"
     },
     " ",
     {
      "type": "underline",
      "text": "under"
     },
     " ",
     {
      "type": "strikethrough",
      "text": "strike"
     },
     " ",
     {
      "type": "spoiler",
      "text": "spoiler"
     },
     " ",
     {
      "type": "code",
      "text": "code"
     },
     " ",
     {
      "type": "marked",
      "text": "marked"
     },
     " ",
     {
      "type": "subscript",
      "text": "sub"
     },
     " ",
     {
      "type": "superscript",
      "text": "sup"
     },
     " ",
     {
      "type": "url",
      "text": "link",
      "url": "https://t.me/"
     },
     " ",
     {
      "type": "email_address",
      "text": "mail",
      "email_address": "user@example.com"
     },
     " ",
     {
      "type": "phone_number",
      "text": "phone",
      "phone_number": "+123456789"
     },
     " ",
     {
      "type": "date_time",
      "text": "22:45 tomorrow",
      "unix_time": 1647531900,
      "date_time_format": "wDT"
     },
     " ",
     {
      "type": "mathematical_expression",
      "expression": "x^2 + y^2"
     },
     " ",
     {
      "type": "hashtag",
      "text": "#hashtag",
      "hashtag": "hashtag"
     },
     " ",
     {
      "type": "cashtag",
      "text": "$USD",
      "cashtag": "USD"
     },
     " ",
     {
      "type": "phone_number",
      "text": "+12345678901",
      "phone_number": "+12345678901"
     },
     ", card: 4242 4242 4242 4242, ",
     {
      "type": "url",
      "text": "https://t.me",
      "url": "https://t.me"
     },
     " ",
     {
      "type": "email_address",
      "text": "a@t.me",
      "email_address": "a@t.me"
     },
     " ",
     {
      "type": "bot_command",
      "text": "/command",
      "bot_command": "command"
     },
     " ",
     {
      "type": "mention",
      "text": "@username",
      "username": "username"
     },
     " ",
     {
      "type": "bold",
      "text": [
       "Bold ",
       {
        "type": "italic",
        "text": [
         "italic ",
         {
          "type": "underline",
          "text": "underlined"
         },
         " italic"
        ]
       },
       " bold"
      ]
     },
     " plain tail"
    ]
   }
  ]
 }
}`

	// rich101cProbeBlocks: probe 02 (02-sendRichMessage-blocks.json).
	rich101cProbeBlocks = `{
 "message_id": 1002,
 "from": {
  "id": 123456789,
  "is_bot": true,
  "first_name": "Test Bot",
  "username": "testbot"
 },
 "chat": {
  "id": -1001234567890,
  "title": "Test Chat",
  "is_forum": true,
  "type": "supergroup"
 },
 "date": 1700000000,
 "rich_message": {
  "blocks": [
   {
    "type": "heading",
    "text": "Heading 1",
    "size": 1
   },
   {
    "type": "heading",
    "text": "Heading 6",
    "size": 6
   },
   {
    "type": "paragraph",
    "text": "Paragraph text"
   },
   {
    "type": "pre",
    "text": "print('x')",
    "language": "python"
   },
   {
    "type": "divider"
   },
   {
    "type": "list",
    "items": [
     {
      "label": "•",
      "blocks": [
       {
        "type": "paragraph",
        "text": "unordered item"
       }
      ]
     },
     {
      "label": "•",
      "blocks": [
       {
        "type": "paragraph",
        "text": "second"
       }
      ]
     }
    ]
   },
   {
    "type": "list",
    "items": [
     {
      "label": "1.",
      "blocks": [
       {
        "type": "paragraph",
        "text": "ordered item"
       }
      ],
      "type": "1",
      "value": 1
     },
     {
      "label": "2.",
      "blocks": [
       {
        "type": "paragraph",
        "text": "ordered two"
       }
      ],
      "type": "1",
      "value": 2
     }
    ]
   },
   {
    "type": "list",
    "items": [
     {
      "label": "•",
      "blocks": [
       {
        "type": "paragraph",
        "text": "task"
       }
      ],
      "has_checkbox": true
     },
     {
      "label": "•",
      "blocks": [
       {
        "type": "paragraph",
        "text": "done"
       }
      ],
      "has_checkbox": true,
      "is_checked": true
     }
    ]
   },
   {
    "type": "blockquote",
    "blocks": [
     {
      "type": "paragraph",
      "text": "Block quotation second line"
     }
    ]
   },
   {
    "type": "table",
    "cells": [
     [
      {
       "text": "H1",
       "is_header": true,
       "align": "left",
       "valign": "middle"
      },
      {
       "text": "H2",
       "is_header": true,
       "align": "center",
       "valign": "middle"
      }
     ],
     [
      {
       "text": "left",
       "align": "left",
       "valign": "middle"
      },
      {
       "text": "center",
       "align": "center",
       "valign": "middle"
      }
     ]
    ],
    "is_bordered": true,
    "is_striped": true
   },
   {
    "type": "paragraph",
    "text": [
     "Text with a reference",
     {
      "type": "superscript",
      "text": [
       {
        "type": "anchor",
        "name": "fnref-id1-1"
       },
       {
        "type": "reference_link",
        "text": "1",
        "reference_name": "fn-id1"
       }
      ]
     },
     "."
    ]
   },
   {
    "type": "mathematical_expression",
    "expression": "E = mc^2"
   },
   {
    "type": "details",
    "summary": [
     "Summary ",
     {
      "type": "bold",
      "text": "bold"
     }
    ],
    "blocks": [
     {
      "type": "heading",
      "text": "Inner heading",
      "size": 3
     },
     {
      "type": "list",
      "items": [
       {
        "label": "•",
        "blocks": [
         {
          "type": "paragraph",
          "text": "inner item"
         }
        ]
       }
      ]
     }
    ],
    "is_open": true
   },
   {
    "type": "footer",
    "text": [
     "1. ",
     {
      "type": "reference",
      "text": "Definition of the footnote.",
      "name": "fn-id1"
     },
     " ",
     {
      "type": "anchor_link",
      "text": "↩",
      "anchor_name": "fnref-id1-1"
     }
    ]
   }
  ]
 }
}`

	// rich101cProbeHTML: probe 03 (03-sendRichMessage-html.json).
	rich101cProbeHTML = `{
 "message_id": 1003,
 "from": {
  "id": 123456789,
  "is_bot": true,
  "first_name": "Test Bot",
  "username": "testbot"
 },
 "chat": {
  "id": -1001234567890,
  "title": "Test Chat",
  "is_forum": true,
  "type": "supergroup"
 },
 "date": 1700000000,
 "rich_message": {
  "blocks": [
   {
    "type": "anchor",
    "name": "chapter-1"
   },
   {
    "type": "paragraph",
    "text": [
     "Go to ",
     {
      "type": "anchor_link",
      "text": "anchor link",
      "anchor_name": "chapter-1"
     },
     "."
    ]
   },
   {
    "type": "pullquote",
    "text": "Pull quote",
    "credit": "The Author"
   },
   {
    "type": "details",
    "summary": "Closed",
    "blocks": [
     {
      "type": "paragraph",
      "text": "Content"
     }
    ]
   },
   {
    "type": "footer",
    "text": "Footer text"
   },
   {
    "type": "map",
    "location": {
     "latitude": 41.900006,
     "longitude": 12.500002
    },
    "zoom": 14,
    "width": 800,
    "height": 400
   }
  ]
 }
}`

	// rich101cProbeMedia: probe 04 (04-sendRichMessage-media.json).
	rich101cProbeMedia = `{
 "message_id": 1004,
 "from": {
  "id": 123456789,
  "is_bot": true,
  "first_name": "Test Bot",
  "username": "testbot"
 },
 "chat": {
  "id": -1001234567890,
  "title": "Test Chat",
  "is_forum": true,
  "type": "supergroup"
 },
 "date": 1700000000,
 "rich_message": {
  "blocks": [
   {
    "type": "photo",
    "photo": [
     {
      "file_id": "FILEID-1",
      "file_unique_id": "UNIQ-1",
      "file_size": 1081,
      "width": 90,
      "height": 51
     },
     {
      "file_id": "FILEID-2",
      "file_unique_id": "UNIQ-2",
      "file_size": 13004,
      "width": 320,
      "height": 180
     },
     {
      "file_id": "FILEID-3",
      "file_unique_id": "UNIQ-3",
      "file_size": 55714,
      "width": 800,
      "height": 450
     },
     {
      "file_id": "FILEID-4",
      "file_unique_id": "UNIQ-4",
      "file_size": 118930,
      "width": 1280,
      "height": 720
     },
     {
      "file_id": "FILEID-5",
      "file_unique_id": "UNIQ-5",
      "file_size": 202060,
      "width": 1920,
      "height": 1080
     }
    ],
    "caption": {
     "text": "Photo caption"
    }
   },
   {
    "type": "collage",
    "blocks": [
     {
      "type": "photo",
      "photo": [
       {
        "file_id": "FILEID-1",
        "file_unique_id": "UNIQ-1",
        "file_size": 1081,
        "width": 90,
        "height": 51
       },
       {
        "file_id": "FILEID-2",
        "file_unique_id": "UNIQ-2",
        "file_size": 13004,
        "width": 320,
        "height": 180
       },
       {
        "file_id": "FILEID-3",
        "file_unique_id": "UNIQ-3",
        "file_size": 55714,
        "width": 800,
        "height": 450
       },
       {
        "file_id": "FILEID-4",
        "file_unique_id": "UNIQ-4",
        "file_size": 118930,
        "width": 1280,
        "height": 720
       },
       {
        "file_id": "FILEID-5",
        "file_unique_id": "UNIQ-5",
        "file_size": 202060,
        "width": 1920,
        "height": 1080
       }
      ]
     },
     {
      "type": "photo",
      "photo": [
       {
        "file_id": "FILEID-1",
        "file_unique_id": "UNIQ-1",
        "file_size": 1081,
        "width": 90,
        "height": 51
       },
       {
        "file_id": "FILEID-2",
        "file_unique_id": "UNIQ-2",
        "file_size": 13004,
        "width": 320,
        "height": 180
       },
       {
        "file_id": "FILEID-3",
        "file_unique_id": "UNIQ-3",
        "file_size": 55714,
        "width": 800,
        "height": 450
       },
       {
        "file_id": "FILEID-4",
        "file_unique_id": "UNIQ-4",
        "file_size": 118930,
        "width": 1280,
        "height": 720
       },
       {
        "file_id": "FILEID-5",
        "file_unique_id": "UNIQ-5",
        "file_size": 202060,
        "width": 1920,
        "height": 1080
       }
      ]
     }
    ]
   }
  ]
 }
}`

	// rich101cProbeRTLPlain: probe 05 (05-sendRichMessage-rtl-plain.json).
	rich101cProbeRTLPlain = `{
 "message_id": 1005,
 "from": {
  "id": 123456789,
  "is_bot": true,
  "first_name": "Test Bot",
  "username": "testbot"
 },
 "chat": {
  "id": -1001234567890,
  "title": "Test Chat",
  "is_forum": true,
  "type": "supergroup"
 },
 "date": 1700000000,
 "rich_message": {
  "blocks": [
   {
    "type": "paragraph",
    "text": "plain only"
   }
  ],
  "is_rtl": true
 }
}`
)
