- **Wrap Errors**: Easily wrap underlying errors with API-specific errors to provide more context.
- **Error Identification**: Check and identify the nature of errors using Go's `errors.Is` and `errors.As` functions.
- **Custom Errors**: Define and use custom errors specific to your API needs.
- **Status Code Handling**: Handle and identify errors based on HTTP status codes.
- **Content Type Handling**: Handle errors based on content type mismatches.

For the code openapi-codegen generates, it also holds:

- **`union`**: decoding and encoding unions and tagged structs.
- **`apitest`**: replaying recorded interactions in generated tests.
- **`page`**: `page.Pages` and `page.Items` yield what an API lists a page at a time, fetching each page as the one
  before is used up, and stopping when the loop breaks or at the first error.
