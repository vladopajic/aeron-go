package aeron

import (
	"github.com/lirm/aeron-go/aeron/buffers"
	"github.com/lirm/aeron-go/aeron/logbuffer"
	"github.com/lirm/aeron-go/aeron/logbuffer/term"
	"github.com/lirm/aeron-go/aeron/util"
)

type ControlledPollFragmentHandler func(buffer *buffers.Atomic, offset int32, length int32, header *logbuffer.Header)

const (
	IMAGE_CLOSED int = -1
)

var ControlledPollAction = struct {
	/**
	 * Abort the current polling operation and do not advance the position for this fragment.
	 */
	ABORT int

	/**
	 * Break from the current polling operation and commit the position as of the end of the current fragment
	 * being handled.
	 */
	BREAK int

	/**
	 * Continue processing but commit the position as of the end of the current fragment so that
	 * flow control is applied to this point.
	 */
	COMMIT int

	/**
	 * Continue processing taking the same approach as the in fragment_handler_t.
	 */
	CONTINUE int
}{
	1,
	2,
	3,
	4,
}

type Image struct {
	termBuffers [logbuffer.PARTITION_COUNT]*buffers.Atomic
	header      logbuffer.Header

	subscriberPosition buffers.Position

	logBuffers *logbuffer.LogBuffers

	sourceIdentity   string
	isClosed         bool // should be atomic
	exceptionHandler func(error)

	correlationId              int64
	subscriptionRegistrationId int64
	sessionId                  int32
	termLengthMask             int32
	positionBitsToShift        uint8
}

func NewImage(sessionId int32, correlationId int64, logBuffers *logbuffer.LogBuffers) *Image {

	image := new(Image)

	image.correlationId = correlationId
	image.sessionId = sessionId
	image.logBuffers = logBuffers
	for i := 0; i < logbuffer.PARTITION_COUNT; i++ {
		image.termBuffers[i] = logBuffers.Buffer(i)
	}
	capacity := int32(logBuffers.Buffer(0).Capacity())
	image.termLengthMask = capacity - 1
	image.positionBitsToShift = util.NumberOfTrailingZeroes(capacity)
	image.header.SetInitialTermId(logbuffer.InitialTermId(logBuffers.Buffer(logbuffer.Descriptor.LOG_META_DATA_SECTION_INDEX)))
	image.header.SetPositionBitsToShift(int32(image.positionBitsToShift))

	return image
}

func (image *Image) Poll(handler term.FragmentHandler, fragmentLimit int) int {

	result := IMAGE_CLOSED

	if !image.isClosed {
		position := image.subscriberPosition.Get()
		//logger.Debugf("Image position: %d, mask:%X", position, image.termLengthMask)
		termOffset := int32(position) & image.termLengthMask
		index := logbuffer.IndexByPosition(position, image.positionBitsToShift)
		termBuffer := image.termBuffers[index]
		//logger.Debugf("Selected Term buffer: %v", termBuffer)

		var readOutcome term.ReadOutcome
		term.Read(&readOutcome, termBuffer, termOffset, handler, fragmentLimit, &image.header)

		newPosition := position + int64(readOutcome.Offset()-termOffset)
		if newPosition > position {
			image.subscriberPosition.Set(newPosition)
			// image.subscriberPosition.setOrdered(newPosition)
		}

		result = readOutcome.FragmentsRead()
	}

	return result
}

func (image *Image) Close() error {
	return image.logBuffers.Close()
}