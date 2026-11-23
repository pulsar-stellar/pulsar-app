// Starts Swagger UI for GET /docs (ADR-059). It lives in its own file, not
// inline, so the page runs under script-src 'self'.
//
// - url: the spec comes from this server, never from a query parameter.
// - supportedSubmitMethods: "Try it out" exists for GET only, so the page
//   never invites anyone to paste the admin token into a form for a write.
// - validatorUrl: null keeps Swagger UI from loading a badge from
//   validator.swagger.io; the CSP would block it anyway.
// - deepLinking: false, so nothing in the URL changes what the page shows.
// - hideAuthorize removes the Authorize button and the per-operation lock
//   icons. Only the write routes take the admin token, and they can't be tried
//   here, so a token field would only invite pasting a secret into a page.
var hideAuthorize = function () {
  var nothing = function () {
    return function () {
      return null;
    };
  };
  return {
    wrapComponents: {
      authorizeBtn: nothing,
      authorizeOperationBtn: nothing,
    },
  };
};

window.addEventListener("load", function () {
  window.ui = SwaggerUIBundle({
    url: "/openapi.json",
    dom_id: "#swagger-ui",
    supportedSubmitMethods: ["get"],
    validatorUrl: null,
    queryConfigEnabled: false,
    deepLinking: false,
    plugins: [hideAuthorize],
  });
});
