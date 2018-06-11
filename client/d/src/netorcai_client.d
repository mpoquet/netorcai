import std.socket;
import std.stdio;
import std.utf;
import std.bitmanip;
import std.json;
import std.format;
import std.exception;

import message_types;
import json_utils;

/// Netorcai metaprotocol client class (D version)
class Client
{
    /// Constructor. Initializes a TCP socket (AF_INET, STREAM)
    this()
    {
        sock = new Socket(AddressFamily.INET, SocketType.STREAM);
    }

    /// Destructor. Closes the socket if needed.
    ~this()
    {
        close();
    }

    /// Connect to a remote endpoint. Throws Exception on error.
    void connect(in string hostname = "localhost", in ushort port = 4242)
    {
        sock.connect(new InternetAddress(hostname, port));
    }

    /// Close the socket.
    void close()
    {
        sock.shutdown(SocketShutdown.BOTH);
        sock.close();
    }

    /// Reads a string message on the client socket. Throws Exception on error.
    string recvString()
    {
        // Read content size
        ubyte[2] contentSizeBuf;
        auto received = sock.receive(contentSizeBuf);
        checkSocketOperation(received, "Cannot read content size.");

        immutable ushort contentSize = littleEndianToNative!ushort(contentSizeBuf);

        // Read content
        ubyte[] contentBuf;
        contentBuf.length = contentSize;
        received = sock.receive(contentBuf);
        checkSocketOperation(received, "Cannot read content.");

        return cast(string) contentBuf;
    }

    /// Reads a JSON message on the client socket. Throws Exception on error.
    JSONValue recvJson()
    {
        return recvString.parseJSON;
    }

    /// Reads a LOGIN_ACK message on the client socket. Throws Exception on error.
    LoginAckMessage readLoginAck()
    {
        auto msg = recvJson();
        switch (msg["message_type"].str)
        {
        case "LOGIN_ACK":
            return LoginAckMessage();
        case "KICK":
            throw new Exception(format!"Kicked from netorai. Reason: %s"(msg["kick_reason"].str));
        default:
            throw new Exception(format!"Unexpected message received: %s"(msg["message_type"].str));
        }
    }

    /// Reads a GAME_STARTS message on the client socket. Throws Exception on error.
    GameStartsMessage readGameStarts()
    {
        auto msg = recvJson;
        switch (msg["message_type"].str)
        {
        case "GAME_STARTS":
            GameStartsMessage m;
            m.playerID = msg["player_id"].getInt;
            m.nbPlayers = msg["nb_players"].getInt;
            m.nbTurnsMax = msg["nb_turns_max"].getInt;
            m.msBeforeFirstTurn = msg["milliseconds_before_first_turn"].getDouble;
            m.msBetweenTurns = msg["milliseconds_between_turns"].getDouble;
            m.initialGameState = msg["initial_game_state"].object;
            m.playersInfo = msg["players_info"].array.parsePlayersInfo;
            return m;
        case "KICK":
            throw new Exception(format!"Kicked from netorai. Reason: %s"(msg["kick_reason"]));
        default:
            throw new Exception(format!"Unexpected message received: %s"(msg["message_type"]));
        }
    }

    /// Reads a TURN message on the client socket. Throws Exception on error.
    TurnMessage readTurn()
    {
        auto msg = recvJson;
        switch (msg["message_type"].str)
        {
        case "TURN":
            TurnMessage m;
            m.turnNumber = msg["turn_number"].getInt;
            m.gameState = msg["game_state"].object;
            m.playersInfo = msg["players_info"].array.parsePlayersInfo;
            return m;
        case "GAME_ENDS":
            throw new Exception("Game over!");
        case "KICK":
            throw new Exception(format!"Kicked from netorai. Reason: %s"(msg["kick_reason"]));
        default:
            throw new Exception(format!"Unexpected message received: %s"(msg["message_type"]));
        }
    }

    /// Send a string message on the client socket. Throws Exception on error.
    void sendString(in string message)
    {
        string content = toUTF8(message ~ "\n");
        ushort contentSize = cast(ushort) content.length;
        ubyte[2] contentSizeBuf = nativeToLittleEndian(contentSize);

        auto sent = sock.send(contentSizeBuf);
        checkSocketOperation(sent, "Cannot send content size.");

        sent = sock.send(content);
        checkSocketOperation(sent, "Cannot send content.");
    }

    /// Send a JSON message on the client socket. Throws Exception on error.
    void sendJson(in JSONValue message)
    {
        sendString(message.toString);
    }

    /// Send a LOGIN message on the client socket. Throws Exception on error.
    void sendLogin(in string nickname, in string role)
    {
        JSONValue msg = ["message_type" : "LOGIN", "nickname" : nickname, "role" : role];

        sendJson(msg);
    }

    /// Send a TURN_ACK message on the client socket. Throws Exception on error.
    void sendTurnAck(in int turnNumber, in JSONValue actions)
    {
        JSONValue msg = ["message_type" : "TURN_ACK"];
        msg.object["turn_number"] = turnNumber;
        msg.object["actions"] = actions;

        sendJson(msg);
    }

    private void checkSocketOperation(in ptrdiff_t result, in string description)
    {
        if (result == Socket.ERROR)
            throw new Exception(description ~ "Socket error.");
        else if (result == 0)
            throw new Exception(description ~ "Socket closed by remote?");
    }

    private Socket sock;
}