package protocol

// ProtocolVersion is the MagiC Protocol (MCP²) version implemented by this build.
// Follows semver: MAJOR.MINOR. Breaking changes bump MAJOR.
//
// Clients that send X-API-Version with a different MAJOR are rejected.
// Clients that send a different MINOR receive a Warning header but are served.
const ProtocolVersion = "1.0"

// APIVersionHeader is the HTTP header clients use to declare their protocol version.
const APIVersionHeader = "X-API-Version"
