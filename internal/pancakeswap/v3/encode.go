package v3

import (
	"encoding/hex"
	"math/big"
)

var (
	tt256   = new(big.Int).Lsh(big.NewInt(1), 256)   // 2 ** 256
	tt256m1 = new(big.Int).Sub(tt256, big.NewInt(1)) // 2 ** 256 - 1
)

func EncodeAddress(addressBytes []byte, size int, left bool) []byte {
	return PadBytes(addressBytes, size, left)
}

func EncodeUint256(n *big.Int, size int, left bool) []byte {
	if n == nil {
		n = big.NewInt(0)
	}
	b := new(big.Int)
	b = b.Set(n)

	if b.Sign() < 0 || b.BitLen() > 256 {
		b.And(b, tt256m1)
	}

	return PadBytes(b.Bytes(), size, left)
}

// use only for hex parameters without 0x
func MustDecodeHex(src string) []byte {
	dst := make([]byte, hex.DecodedLen(len([]byte(src))))
	n, err := hex.Decode(dst, []byte(src))
	if err != nil {
		panic(err)
	}

	return dst[:n]
}

func PadBytes(b []byte, size int, left bool) []byte {
	l := len(b)
	if l == size {
		return b
	}
	if l > size {
		return b[l-size:]
	}
	tmp := make([]byte, size)
	if left {
		copy(tmp[size-l:], b)
	} else {
		copy(tmp, b)
	}
	return tmp
}
