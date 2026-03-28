# Google Docs hunting dataset

TODO:
* Filter out or add support for truncated Google Docs documents.

```text
2026/03/26 19:17:54 11Wp7VnWXPZ-DPNbeFkw2VzT2NLdzEQNU: error: json unmarshall `"\u0003\n\n\n\n\u0003Authorshi...`: unexpected end of JSON input
2026/03/26 19:21:45 19REL2RZM0exwcuPKhZZ_ygh7ypMDlbMQhvgqwE2KPi0: error: json unmarshall `"English       Все возм...`: unexpected end of JSON input
```

ISSUES:

* [x] received error 403 - implemented rate limiting
* sqlite inserted binary instead of text (0B5b-tmb2-DI3djg5dUMzbkNFR0k)
* gdocs missed content (0B5LPyQqaZw-3cXRBT0kyNTh2TVU)
