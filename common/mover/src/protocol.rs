/// Low-level mover protocol - simple binary format for UDS communication
/// Provides primitive operations: READ, WRITE, SEND_ZC, RECV

use std::io::{self, Read, Write};

/// Operation codes for mover requests
#[repr(u8)]
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum OpCode {
    /// Read data from CAS (mmap)
    Read = 0x01,
    /// Write data to CAS (write-through)
    Write = 0x02,
    /// Zero-copy send to peer
    SendZC = 0x03,
    /// Receive into registered buffer
    Recv = 0x04,
    /// Batch multiple operations
    Batch = 0x05,
}

impl TryFrom<u8> for OpCode {
    type Error = io::Error;

    fn try_from(value: u8) -> Result<Self, Self::Error> {
        match value {
            0x01 => Ok(OpCode::Read),
            0x02 => Ok(OpCode::Write),
            0x03 => Ok(OpCode::SendZC),
            0x04 => Ok(OpCode::Recv),
            0x05 => Ok(OpCode::Batch),
            _ => Err(io::Error::new(io::ErrorKind::InvalidData, "unknown op code")),
        }
    }
}

/// Mover request (generic, no workflow knowledge)
#[derive(Debug)]
pub struct MoverRequest {
    pub op: OpCode,
    pub id: Vec<u8>,     // cas_id or peer_id
    pub offset: usize,   // For SEND_ZC
    pub length: usize,   // Data length
    pub data: Vec<u8>,   // Payload (for WRITE)
}

impl MoverRequest {
    /// Serialize request to binary format
    /// Format: [op: u8][id_len: u16][id: bytes][offset: u64][len: u64][data_len: u32][data: bytes]
    pub fn write_to<W: Write>(&self, writer: &mut W) -> io::Result<()> {
        // Write op code
        writer.write_all(&[self.op as u8])?;

        // Write ID (variable length)
        writer.write_all(&(self.id.len() as u16).to_le_bytes())?;
        writer.write_all(&self.id)?;

        // Write offset and length
        writer.write_all(&self.offset.to_le_bytes())?;
        writer.write_all(&self.length.to_le_bytes())?;

        // Write data (variable length)
        writer.write_all(&(self.data.len() as u32).to_le_bytes())?;
        writer.write_all(&self.data)?;

        writer.flush()
    }

    /// Deserialize request from binary format
    pub fn read_from<R: Read>(reader: &mut R) -> io::Result<Self> {
        // Read op code
        let mut op_byte = [0u8; 1];
        reader.read_exact(&mut op_byte)?;
        let op = OpCode::try_from(op_byte[0])?;

        // Read ID
        let mut id_len_bytes = [0u8; 2];
        reader.read_exact(&mut id_len_bytes)?;
        let id_len = u16::from_le_bytes(id_len_bytes) as usize;
        let mut id = vec![0u8; id_len];
        reader.read_exact(&mut id)?;

        // Read offset and length
        let mut offset_bytes = [0u8; 8];
        reader.read_exact(&mut offset_bytes)?;
        let offset = usize::from_le_bytes(offset_bytes);

        let mut len_bytes = [0u8; 8];
        reader.read_exact(&mut len_bytes)?;
        let length = usize::from_le_bytes(len_bytes);

        // Read data
        let mut data_len_bytes = [0u8; 4];
        reader.read_exact(&mut data_len_bytes)?;
        let data_len = u32::from_le_bytes(data_len_bytes) as usize;
        let mut data = vec![0u8; data_len];
        reader.read_exact(&mut data)?;

        Ok(MoverRequest {
            op,
            id,
            offset,
            length,
            data,
        })
    }
}

/// Mover response
#[derive(Debug)]
pub struct MoverResponse {
    pub status: ResponseStatus,
    pub data: Vec<u8>,
}

#[repr(u8)]
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ResponseStatus {
    Ok = 0x00,
    NotFound = 0x01,
    Error = 0x02,
}

impl MoverResponse {
    /// Serialize response to binary format
    /// Format: [status: u8][data_len: u32][data: bytes]
    pub fn write_to<W: Write>(&self, writer: &mut W) -> io::Result<()> {
        writer.write_all(&[self.status as u8])?;
        writer.write_all(&(self.data.len() as u32).to_le_bytes())?;
        writer.write_all(&self.data)?;
        writer.flush()
    }

    /// Deserialize response from binary format
    pub fn read_from<R: Read>(reader: &mut R) -> io::Result<Self> {
        let mut status_byte = [0u8; 1];
        reader.read_exact(&mut status_byte)?;

        let status = match status_byte[0] {
            0x00 => ResponseStatus::Ok,
            0x01 => ResponseStatus::NotFound,
            0x02 => ResponseStatus::Error,
            _ => return Err(io::Error::new(io::ErrorKind::InvalidData, "unknown status")),
        };

        let mut data_len_bytes = [0u8; 4];
        reader.read_exact(&mut data_len_bytes)?;
        let data_len = u32::from_le_bytes(data_len_bytes) as usize;

        let mut data = vec![0u8; data_len];
        reader.read_exact(&mut data)?;

        Ok(MoverResponse { status, data })
    }

    pub fn ok(data: Vec<u8>) -> Self {
        Self {
            status: ResponseStatus::Ok,
            data,
        }
    }

    pub fn not_found() -> Self {
        Self {
            status: ResponseStatus::NotFound,
            data: Vec::new(),
        }
    }

    pub fn error(message: String) -> Self {
        Self {
            status: ResponseStatus::Error,
            data: message.into_bytes(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Cursor;

    #[test]
    fn test_request_roundtrip() {
        let req = MoverRequest {
            op: OpCode::Read,
            id: b"test-cas-id".to_vec(),
            offset: 0,
            length: 1024,
            data: Vec::new(),
        };

        let mut buffer = Vec::new();
        req.write_to(&mut buffer).unwrap();

        let mut cursor = Cursor::new(buffer);
        let decoded = MoverRequest::read_from(&mut cursor).unwrap();

        assert_eq!(decoded.op, OpCode::Read);
        assert_eq!(decoded.id, b"test-cas-id");
    }

    #[test]
    fn test_response_roundtrip() {
        let resp = MoverResponse::ok(b"test-data".to_vec());

        let mut buffer = Vec::new();
        resp.write_to(&mut buffer).unwrap();

        let mut cursor = Cursor::new(buffer);
        let decoded = MoverResponse::read_from(&mut cursor).unwrap();

        assert_eq!(decoded.status, ResponseStatus::Ok);
        assert_eq!(decoded.data, b"test-data");
    }
}
