package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	graphql "github.com/graph-gophers/graphql-go"
	gqlerrors "github.com/graph-gophers/graphql-go/errors"

	"github.com/pulsar-stellar/pulsar-app/indexer/internal/apierror"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/models"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/store"
	"github.com/pulsar-stellar/pulsar-app/indexer/internal/validate"
)

// maxGraphQLBodyBytes caps the GraphQL request body the handler will read. A
// query document plus its variables is small, so the cap matches
// maxRegisterBodyBytes while still bounding what an abusive client can make the
// handler buffer, per ADR-043.
const maxGraphQLBodyBytes = 64 << 10 // 64 KiB

// graphQLSchemaSDL is the read-only GraphQL schema. It is GraphQL-idiomatic
// camelCase and carries no Mutation: register and delete stay on the REST
// surface and pick up auth at step 72, so GraphQL exposes only reads (ADR-043).
//
// Two scalars are custom. Time is graph-gophers' built-in RFC3339 scalar,
// declared here because it is not predeclared. JSON is the indexer's own
// output-only scalar carrying the ADR-023 decoded topic and data taxonomy as
// raw bytes. Every id is an ID serialized as a decimal string, since a
// BIGSERIAL event id can outgrow a JSON number's safe range (ADR-021).
const graphQLSchemaSDL = `
schema {
	query: Query
}

scalar Time
scalar JSON

type Query {
	health: Health!
	contracts: [Contract!]!
	contract(id: ID!): Contract
	event(id: ID!): Event
	events(
		contractId: ID!
		name: String
		fromLedger: Int
		toLedger: Int
		topicContains: String
		limit: Int
		cursor: String
		order: String
	): EventConnection!
}

type Health {
	ok: Boolean!
	version: String!
	latestLedger: Int!
	trackedContracts: Int!
}

type Contract {
	id: ID!
	addedAt: Time!
	firstIndexedLedger: Int
	lastIndexedLedger: Int!
	status: String!
	events(
		name: String
		fromLedger: Int
		toLedger: Int
		topicContains: String
		limit: Int
		cursor: String
		order: String
	): EventConnection!
}

type Event {
	id: ID!
	contractId: ID!
	ledger: Int!
	txHash: String!
	eventIndex: Int!
	name: String!
	topics: JSON!
	data: JSON!
	rawTopics: [String!]!
	rawData: String!
	emittedAt: Time!
	inSuccessfulContractCall: Boolean!
}

type EventConnection {
	items: [Event!]!
	nextCursor: String
}
`

// newGraphQLSchema parses the read-only schema against a root resolver that
// closes over s, so every resolver reaches the exact store interfaces the REST
// handlers use. It is called once from NewServer; the parsed schema is
// concurrency-safe for the many requests that share it.
//
// The security limits ride here as schema options, per ADR-043: MaxDepth bounds
// query nesting well above the schema's real depth of about four, MaxQueryLength
// bounds the document a client can send, and MaxParallelism caps the concurrent
// store reads a single query can fan out to. A PanicHandler keeps a recovered
// resolver panic from leaking its value to the client. Introspection stays
// enabled: the schema is public and documented, and carries no secret.
//
// A parse failure is a programming error in the static SDL, not a runtime
// condition, so it panics at construction where the first test catches it,
// never on a request path.
func newGraphQLSchema(s *Server) *graphql.Schema {
	opts := []graphql.SchemaOpt{
		graphql.MaxDepth(10),
		graphql.MaxQueryLength(8192),
		graphql.MaxParallelism(10),
		graphql.PanicHandler(graphQLPanicHandler{s: s}),
	}
	return graphql.MustParseSchema(graphQLSchemaSDL, &queryResolver{s: s}, opts...)
}

// graphQLPanicHandler turns a panic recovered inside query execution into the
// generic INTERNAL_PANIC resolver error. graph-gophers recovers a resolver
// panic itself, inside the execution goroutine, before the Recoverer middleware
// that wraps the mux can ever see it, and its default handler would render the
// recovered value straight into errors[].message. That would bypass the no-leak
// discipline the resolverError adapter enforces everywhere else, so this path
// gets the same guard: the recovered value can name internal types, so it is
// logged server-side against the request and never sent (ADR-043). No resolver
// is expected to panic; this is defense in depth, and it reuses the catalogued
// INTERNAL_PANIC code the recovery middleware already uses, adding no new code.
type graphQLPanicHandler struct {
	s *Server
}

// MakePanicError logs the recovered value and returns the generic panic error,
// carrying only the catalog code and wire class in extensions, the same shape
// resolverError produces.
func (h graphQLPanicHandler) MakePanicError(ctx context.Context, value any) *gqlerrors.QueryError {
	h.s.log.ErrorContext(ctx, "graphql_resolver_panic",
		"request_id", RequestIDFromContext(ctx),
		"panic", value,
	)
	e := apierror.New(apierror.CodeInternalPanic, "The indexer hit an unexpected condition while executing the query.")
	return &gqlerrors.QueryError{
		Message: e.Message(),
		Extensions: map[string]any{
			"code":  string(e.Code()),
			"class": string(e.Class()),
		},
	}
}

// JSON is the output-only GraphQL scalar for an event's decoded topics and
// data. It carries the ADR-023 taxonomy as the raw bytes the store already
// holds, so marshaling passes them through untouched rather than round-tripping
// through a map.
//
// graph-gophers requires every scalar's Go type to satisfy its Unmarshaler
// interface even when the scalar is never an input, so UnmarshalGraphQL exists
// only to reject that use: the schema exposes JSON on output types alone, and a
// client that tries to send one gets a clear error rather than a silent decode.
type JSON struct {
	raw json.RawMessage
}

// ImplementsGraphQLType registers this Go type as the schema's JSON scalar.
func (JSON) ImplementsGraphQLType(name string) bool { return name == "JSON" }

// UnmarshalGraphQL rejects JSON as an input: the scalar is output-only, so no
// client-supplied bytes are ever decoded through it.
func (j *JSON) UnmarshalGraphQL(any) error {
	return errors.New("the JSON scalar is output-only and cannot be used as an input")
}

// MarshalJSON emits the stored bytes verbatim, or null when there are none, so
// the decoded taxonomy reaches the client exactly as the store recorded it.
func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j.raw) == 0 {
		return []byte("null"), nil
	}
	return j.raw, nil
}

// resolverError adapts a catalogued apierror.Error into the error a resolver
// returns, so graph-gophers renders the catalog code and wire class into the
// GraphQL error's extensions while the message stays the generic client-facing
// text. The underlying cause is never carried here: a store failure is logged
// server-side and this value carries only apierror's generic message, so
// nothing internal leaks into the GraphQL error (ADR-043).
type resolverError struct{ err *apierror.Error }

// Error returns the generic client-facing message, never the wrapped cause, so
// the GraphQL errors[].message stays free of internal detail.
func (e resolverError) Error() string { return e.err.Message() }

// Extensions carries the catalog code and wire class into errors[].extensions,
// giving a GraphQL client the same finer-grained signal ADR-038 puts in the
// REST envelope's error.details.code.
func (e resolverError) Extensions() map[string]any {
	return map[string]any{
		"code":  string(e.err.Code()),
		"class": string(e.err.Class()),
	}
}

// internalStoreError logs the real cause server-side against the request and
// returns the generic INTERNAL_STORE resolver error, the GraphQL counterpart of
// the 500 the REST handlers return. The cause can name a host or carry store
// internals, so it is logged and never sent, matching handleListEvents.
func (s *Server) internalStoreError(ctx context.Context, msg string, cause error) error {
	s.log.ErrorContext(ctx, msg,
		"request_id", RequestIDFromContext(ctx),
		"error", cause.Error(),
	)
	return resolverError{err: apierror.New(apierror.CodeInternalStore, "The indexer could not reach its store to answer the request.")}
}

// queryResolver is the GraphQL Query root. It closes over the Server so every
// field reaches the same contractStore and eventStore the REST handlers use: no
// new store method, no new SQL, every read through the existing parameterized
// store (ADR-043).
type queryResolver struct {
	s *Server
}

// Health resolves the Query.health field, reporting liveness and how far the
// indexer has progressed. A store failure is an INTERNAL_STORE resolver error
// with the cause logged, mirroring handleHealth.
func (q *queryResolver) Health(ctx context.Context) (*healthResolver, error) {
	stats, err := q.s.contracts.Stats(ctx)
	if err != nil {
		return nil, q.s.internalStoreError(ctx, "graphql_health_stats_failed", err)
	}
	return &healthResolver{version: q.s.version, stats: stats}, nil
}

// Contracts resolves Query.contracts with every tracked contract, oldest first,
// the same read handleListContracts serves. A store failure is INTERNAL_STORE.
func (q *queryResolver) Contracts(ctx context.Context) ([]*contractResolver, error) {
	contracts, err := q.s.contracts.List(ctx)
	if err != nil {
		return nil, q.s.internalStoreError(ctx, "graphql_contracts_list_failed", err)
	}
	out := make([]*contractResolver, 0, len(contracts))
	for i := range contracts {
		out = append(out, &contractResolver{s: q.s, c: contracts[i]})
	}
	return out, nil
}

// Contract resolves Query.contract(id:). A malformed id is a VALIDATION_CONTRACT_ID
// resolver error; a well-formed id the indexer is not tracking resolves to
// GraphQL null, the idiomatic "looked up, absent" that mirrors the SDK's
// REST-to-null mapping (ADR-043). Any other store failure is INTERNAL_STORE.
func (q *queryResolver) Contract(ctx context.Context, args struct{ ID graphql.ID }) (*contractResolver, error) {
	id := string(args.ID)
	if err := validate.ContractID(id); err != nil {
		return nil, resolverError{err: apierror.New(apierror.CodeValidationContractID, "The contract ID is not a well-formed Soroban contract ID.")}
	}
	contract, err := q.s.contracts.Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, q.s.internalStoreError(ctx, "graphql_contract_get_failed", err)
	}
	return &contractResolver{s: q.s, c: contract}, nil
}

// Event resolves Query.event(id:). A non-numeric or out-of-range id is a
// VALIDATION_EVENT_ID resolver error; a well-formed id that finds nothing
// resolves to GraphQL null, distinct from the malformed case (ADR-021,
// ADR-043). Any other store failure is INTERNAL_STORE.
func (q *queryResolver) Event(ctx context.Context, args struct{ ID graphql.ID }) (*eventResolver, error) {
	id, err := validate.EventID(string(args.ID))
	if err != nil {
		return nil, resolverError{err: apierror.New(apierror.CodeValidationEventID, "The event ID must be a non-negative integer.")}
	}
	event, err := q.s.events.Get(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		return nil, q.s.internalStoreError(ctx, "graphql_event_get_failed", err)
	}
	return &eventResolver{e: event}, nil
}

// Events resolves Query.events(contractId:, ...). The contract id and every
// filter are validated at the boundary before any store read, so an invalid
// argument is a catalogued resolver error rather than a store error mapped to
// 500. Like handleListEvents it then verifies the contract is tracked: an
// untracked contract is a NOT_FOUND_CONTRACT resolver error, not an empty
// connection, so the ADR-021 distinction between "untracked" and "tracked, no
// matches" survives. A tracked contract with no matches resolves to items: [].
func (q *queryResolver) Events(ctx context.Context, args eventsArgs) (*eventConnectionResolver, error) {
	contractID := string(args.ContractID)
	if err := validate.ContractID(contractID); err != nil {
		return nil, resolverError{err: apierror.New(apierror.CodeValidationContractID, "The contract ID is not a well-formed Soroban contract ID.")}
	}
	query, apiErr := buildEventQuery(contractID, args.eventFilterArgs)
	if apiErr != nil {
		return nil, resolverError{err: apiErr}
	}

	if _, err := q.s.contracts.Get(ctx, contractID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, resolverError{err: apierror.New(apierror.CodeNotFoundContract, "The indexer is not tracking this contract.")}
		}
		return nil, q.s.internalStoreError(ctx, "graphql_events_contract_get_failed", err)
	}

	page, err := q.s.events.Query(ctx, query)
	if err != nil {
		return nil, q.s.internalStoreError(ctx, "graphql_events_query_failed", err)
	}
	return &eventConnectionResolver{events: page.Events, nextCursor: page.NextCursor}, nil
}

// healthResolver resolves the Health type from a version string and the store's
// aggregate. The two counts are int64 and int in Go but protocol-bounded, so
// they narrow to int32 for GraphQL Int, safe within Stellar's ledger range.
type healthResolver struct {
	version string
	stats   store.ContractStats
}

func (r *healthResolver) OK() bool                { return true }
func (r *healthResolver) Version() string         { return r.version }
func (r *healthResolver) LatestLedger() int32     { return int32(r.stats.LatestLedger) }
func (r *healthResolver) TrackedContracts() int32 { return int32(r.stats.Count) }

// contractResolver resolves the Contract type. It holds the Server so its
// nested events field reaches the same stores as the top-level query.
type contractResolver struct {
	s *Server
	c models.Contract
}

func (r *contractResolver) ID() graphql.ID           { return graphql.ID(r.c.ID) }
func (r *contractResolver) AddedAt() graphql.Time    { return graphql.Time{Time: r.c.AddedAt} }
func (r *contractResolver) LastIndexedLedger() int32 { return int32(r.c.LastIndexedLedger) }
func (r *contractResolver) Status() string           { return string(r.c.Status) }

// FirstIndexedLedger is nullable: null means the first poll has not completed,
// a distinct claim from ledger zero (see models.Contract), so a nil pointer
// resolves to GraphQL null rather than 0.
func (r *contractResolver) FirstIndexedLedger() *int32 {
	if r.c.FirstIndexedLedger == nil {
		return nil
	}
	v := int32(*r.c.FirstIndexedLedger)
	return &v
}

// Events resolves Contract.events, the nesting the REST surface cannot express.
// It skips the tracked check the top-level events field runs: the parent
// contract already resolved, so it is known tracked. Filters are validated the
// same way, and a store failure is INTERNAL_STORE.
func (r *contractResolver) Events(ctx context.Context, args eventFilterArgs) (*eventConnectionResolver, error) {
	query, apiErr := buildEventQuery(r.c.ID, args)
	if apiErr != nil {
		return nil, resolverError{err: apiErr}
	}
	page, err := r.s.events.Query(ctx, query)
	if err != nil {
		return nil, r.s.internalStoreError(ctx, "graphql_events_query_failed", err)
	}
	return &eventConnectionResolver{events: page.Events, nextCursor: page.NextCursor}, nil
}

// eventResolver resolves the Event type. The id narrows to a decimal ID string
// (ADR-021); ledger and eventIndex narrow to int32, safe within the uint32
// protocol bound; topics and data pass through as the JSON scalar.
type eventResolver struct {
	e *models.Event
}

func (r *eventResolver) ID() graphql.ID                 { return graphql.ID(strconv.FormatInt(r.e.ID, 10)) }
func (r *eventResolver) ContractID() graphql.ID         { return graphql.ID(r.e.ContractID) }
func (r *eventResolver) Ledger() int32                  { return int32(r.e.Ledger) }
func (r *eventResolver) TxHash() string                 { return r.e.TxHash }
func (r *eventResolver) EventIndex() int32              { return int32(r.e.EventIndex) }
func (r *eventResolver) Name() string                   { return r.e.Name }
func (r *eventResolver) Topics() JSON                   { return JSON{raw: r.e.TopicsJSON} }
func (r *eventResolver) Data() JSON                     { return JSON{raw: r.e.DataJSON} }
func (r *eventResolver) RawData() string                { return r.e.RawData }
func (r *eventResolver) EmittedAt() graphql.Time        { return graphql.Time{Time: r.e.EmittedAt} }
func (r *eventResolver) InSuccessfulContractCall() bool { return r.e.InSuccessfulContractCall }

// RawTopics coerces a nil slice to an empty one: the schema field is a non-null
// list, so null would be a GraphQL error, and "no raw topics" is [].
func (r *eventResolver) RawTopics() []string {
	if r.e.RawTopics == nil {
		return []string{}
	}
	return r.e.RawTopics
}

// eventConnectionResolver resolves the EventConnection type: a page of events
// and the cursor for the next page, mirroring the REST paged envelope.
type eventConnectionResolver struct {
	events     []*models.Event
	nextCursor *string
}

// Items resolves the non-null list, coercing a nil page to an empty slice so a
// tracked contract with no matches serializes as [] rather than null.
func (r *eventConnectionResolver) Items() []*eventResolver {
	out := make([]*eventResolver, 0, len(r.events))
	for _, e := range r.events {
		out = append(out, &eventResolver{e: e})
	}
	return out
}

// NextCursor resolves the nullable cursor: nil when the page is exhausted.
func (r *eventConnectionResolver) NextCursor() *string { return r.nextCursor }

// eventFilterArgs is the set of optional event filters shared by Query.events
// and Contract.events. Each is a pointer so an omitted argument is nil, which
// the validators read as absent and default per the SDK's EventQuerySchema.
type eventFilterArgs struct {
	Name          *string
	FromLedger    *int32
	ToLedger      *int32
	TopicContains *string
	Limit         *int32
	Cursor        *string
	Order         *string
}

// eventsArgs is Query.events' arguments: the required contract id plus the
// shared filters. ContractID is non-pointer because the schema marks it ID!,
// which graph-gophers packs into a non-pointer field.
type eventsArgs struct {
	ContractID graphql.ID
	eventFilterArgs
}

// buildEventQuery turns typed GraphQL filter arguments into a store.EventQuery,
// validating each through internal/validate, the single SDK-mirrored authority
// the REST path uses. Each typed argument is formatted back into the string
// form the validators expect and run through the same function parseEventQuery
// calls, so the GraphQL surface inherits every SDK default (limit 50, order
// desc) and every rejection (limit 0, a negative ledger, order "up") for free:
// the digit patterns reject a sign or non-digit exactly as they do a query
// string. A rejection returns the catalogued apierror the caller wraps.
func buildEventQuery(contractID string, f eventFilterArgs) (store.EventQuery, *apierror.Error) {
	limit, err := validate.EventLimit(int32PtrString(f.Limit))
	if err != nil {
		return store.EventQuery{}, apierror.New(apierror.CodeValidationLimit, "The limit argument must be an integer between 1 and 500.")
	}
	order, err := validate.EventOrder(strPtrString(f.Order))
	if err != nil {
		return store.EventQuery{}, apierror.New(apierror.CodeValidationOrder, "The order argument must be asc or desc.")
	}
	cursor, err := validate.EventCursor(strPtrString(f.Cursor))
	if err != nil {
		return store.EventQuery{}, apierror.New(apierror.CodeValidationCursor, "The cursor argument must be an event id.")
	}
	fromLedger, err := validate.LedgerBound(int32PtrString(f.FromLedger))
	if err != nil {
		return store.EventQuery{}, apierror.New(apierror.CodeValidationLedgerRange, "The fromLedger argument must be a non-negative integer.")
	}
	toLedger, err := validate.LedgerBound(int32PtrString(f.ToLedger))
	if err != nil {
		return store.EventQuery{}, apierror.New(apierror.CodeValidationLedgerRange, "The toLedger argument must be a non-negative integer.")
	}
	if err := validate.LedgerRange(fromLedger, toLedger); err != nil {
		return store.EventQuery{}, apierror.New(apierror.CodeValidationLedgerRange, "The fromLedger argument must not be greater than toLedger.")
	}
	name, err := validate.EventFilterText(strPtrString(f.Name))
	if err != nil {
		return store.EventQuery{}, apierror.New(apierror.CodeValidationFilter, "The name argument must be valid UTF-8 and must not contain a NUL byte.")
	}
	topicContains, err := validate.EventFilterText(strPtrString(f.TopicContains))
	if err != nil {
		return store.EventQuery{}, apierror.New(apierror.CodeValidationFilter, "The topicContains argument must be valid UTF-8 and must not contain a NUL byte.")
	}

	return store.EventQuery{
		ContractID:    contractID,
		Name:          name,
		FromLedger:    fromLedger,
		ToLedger:      toLedger,
		Limit:         limit,
		Cursor:        cursor,
		Order:         order,
		TopicContains: topicContains,
	}, nil
}

// strPtrString renders an optional string argument as the value the validators
// read: an omitted argument becomes the empty string, which each validator
// treats as absent.
func strPtrString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// int32PtrString renders an optional Int argument as the decimal string the
// validators parse: an omitted argument becomes the empty string, a present one
// its base-10 form, so a negative or zero value reaches the validator's digit
// pattern and is rejected there.
func int32PtrString(p *int32) string {
	if p == nil {
		return ""
	}
	return strconv.FormatInt(int64(*p), 10)
}

// graphQLRequest is the POST /graphql body per the GraphQL-over-HTTP JSON
// shape: the query document, an optional operation name to select one of
// several operations, and optional variables.
type graphQLRequest struct {
	Query         string         `json:"query"`
	OperationName string         `json:"operationName"`
	Variables     map[string]any `json:"variables"`
}

// handleGraphQL answers POST /graphql. It bounds and decodes the body, then
// hands the query to the shared schema and writes the spec-fixed { data, errors }
// envelope, distinct from the ADR-017 envelope the REST routes use (ADR-043).
//
// Execution outcomes, including field-level resolver errors, ride in the
// response body at HTTP 200 per GraphQL over HTTP: a client reads errors[], not
// the status. Only a transport-level failure the schema never sees, a body that
// is too large, malformed, or carries no query, returns 400 with a
// GraphQL-shaped error. The handler is mounted inside RequestLogger and
// Recoverer, so it inherits request-id logging and panic recovery.
func (s *Server) handleGraphQL(w http.ResponseWriter, r *http.Request) {
	var req graphQLRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxGraphQLBodyBytes))
	if err := dec.Decode(&req); err != nil {
		writeGraphQLTransportError(w, `The request body must be a JSON object of the form {"query": "..."}.`)
		return
	}
	if strings.TrimSpace(req.Query) == "" {
		writeGraphQLTransportError(w, "The request must carry a non-empty GraphQL query.")
		return
	}

	resp := s.schema.Exec(r.Context(), req.Query, req.OperationName, req.Variables)
	out, err := json.Marshal(resp)
	if err != nil {
		s.log.ErrorContext(r.Context(), "graphql_marshal_failed",
			"request_id", RequestIDFromContext(r.Context()),
			"error", err.Error(),
		)
		writeGraphQLTransportError(w, "The indexer could not encode the GraphQL response.")
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
}

// writeGraphQLTransportError writes a 400 carrying a GraphQL-shaped
// { errors: [{ message }] } body, for the transport-level failures the schema
// never executes: a body too large, malformed, or missing its query. It stays
// GraphQL-shaped rather than borrowing the ADR-017 envelope so a GraphQL client
// parses it the same way it parses an execution error.
func writeGraphQLTransportError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"errors": []map[string]any{{"message": message}},
	})
}
